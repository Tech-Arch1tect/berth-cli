package cmd

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	"github.com/Tech-Arch1tect/berth-cli/pkg/ws"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var (
	dockerServices []string
	dockerOptions  []string
	dockerFollow   bool
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Execute Docker Compose operations on stacks",
	Long: `Execute Docker Compose operations on remote stacks via a Berth server.

Supports up, down, start, stop, restart, and pull commands. Operations are
streamed in real-time over WebSocket by default. Use --follow=false to start
an operation without waiting for output.

Examples:
  # Start a stack
  berth-cli docker up -s 1 -n my-stack

  # Pull images for specific services
  berth-cli docker pull -s 1 -n my-stack --services web,api

  # Restart without following logs
  berth-cli docker restart -s 1 -n my-stack --follow=false

  # Stop with additional docker compose options
  berth-cli docker down -s 1 -n my-stack --options "--remove-orphans"`,
}

func init() {
	rootCmd.AddCommand(dockerCmd)

	addServerIDFlag(dockerCmd, true)
	addStackFlag(dockerCmd, true)
	dockerCmd.PersistentFlags().StringSliceVar(&dockerServices, "services", []string{}, "Specific services to target (comma-separated)")
	dockerCmd.PersistentFlags().StringSliceVar(&dockerOptions, "options", []string{}, "Additional options to pass to docker compose")
	dockerCmd.PersistentFlags().BoolVarP(&dockerFollow, "follow", "f", true, "Follow operation logs (default: true)")

	dockerCmd.AddCommand(
		&cobra.Command{
			Use:   "up",
			Short: "Start a stack or specific services",
			Long: `Start a Docker stack using 'docker compose up -d'.

Use --services to target specific services, or omit to start the entire stack.

Examples:
  # Start the entire stack
  berth-cli docker up -s 1 -n my-stack

  # Start specific services only
  berth-cli docker up -s 1 -n my-stack --services web,db

  # Start and force recreate containers
  berth-cli docker up -s 1 -n my-stack --options "--force-recreate"

  # Start without following logs
  berth-cli docker up -s 1 -n my-stack --follow=false`,
			RunE: runDockerOperation("up"),
		},
		&cobra.Command{
			Use:   "down",
			Short: "Stop and remove a stack or specific services",
			Long: `Stop and remove containers, networks, and resources for a Docker stack using 'docker compose down'.

Use --services to target specific services, or omit to tear down the entire stack.

Examples:
  # Stop and remove the entire stack
  berth-cli docker down -s 1 -n my-stack

  # Remove with orphan cleanup
  berth-cli docker down -s 1 -n my-stack --options "--remove-orphans"

  # Remove including volumes
  berth-cli docker down -s 1 -n my-stack --options "--volumes"`,
			RunE: runDockerOperation("down"),
		},
		&cobra.Command{
			Use:   "start",
			Short: "Start existing stopped containers",
			Long: `Start existing stopped containers for a stack using 'docker compose start'.

Unlike 'up', this only starts containers that already exist but are stopped.
It does not create new containers.

Examples:
  # Start all stopped containers in the stack
  berth-cli docker start -s 1 -n my-stack

  # Start a specific service
  berth-cli docker start -s 1 -n my-stack --services web`,
			RunE: runDockerOperation("start"),
		},
		&cobra.Command{
			Use:   "stop",
			Short: "Stop running containers without removing them",
			Long: `Stop running containers for a stack using 'docker compose stop'.

Unlike 'down', this only stops containers without removing them or their
networks. Containers can be restarted with 'start'.

Examples:
  # Stop all containers in the stack
  berth-cli docker stop -s 1 -n my-stack

  # Stop a specific service
  berth-cli docker stop -s 1 -n my-stack --services db

  # Stop with a custom timeout
  berth-cli docker stop -s 1 -n my-stack --options "-t","30"`,
			RunE: runDockerOperation("stop"),
		},
		&cobra.Command{
			Use:   "restart",
			Short: "Restart running containers",
			Long: `Restart containers for a stack using 'docker compose restart'.

Examples:
  # Restart all containers in the stack
  berth-cli docker restart -s 1 -n my-stack

  # Restart a specific service
  berth-cli docker restart -s 1 -n my-stack --services web

  # Restart without following logs
  berth-cli docker restart -s 1 -n my-stack --follow=false`,
			RunE: runDockerOperation("restart"),
		},
		&cobra.Command{
			Use:   "pull",
			Short: "Pull latest images for a stack",
			Long: `Pull the latest container images for a stack using 'docker compose pull'.

Examples:
  # Pull all images for the stack
  berth-cli docker pull -s 1 -n my-stack

  # Pull images for specific services only
  berth-cli docker pull -s 1 -n my-stack --services web,api

  # Pull without following logs
  berth-cli docker pull -s 1 -n my-stack --follow=false`,
			RunE: runDockerOperation("pull"),
		},
	)
}

type operationRequest struct {
	Command  string   `json:"command"`
	Options  []string `json:"options"`
	Services []string `json:"services"`
}

type operationResponse struct {
	OperationID string `json:"operationId"`
}

type streamMessage struct {
	Type      string    `json:"type"`
	Data      string    `json:"data,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Success   *bool     `json:"success,omitempty"`
	ExitCode  *int      `json:"exitCode,omitempty"`
}

func runDockerOperation(command string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(cmd)
		if err != nil {
			return err
		}

		serverID := getServerIDStr(cmd)
		stackName := getStackName(cmd)

		operationID, err := startOperation(cfg, serverID, stackName, operationRequest{
			Command:  command,
			Options:  dockerOptions,
			Services: dockerServices,
		})
		if err != nil {
			return fmt.Errorf("failed to start operation: %w", err)
		}

		fmt.Printf("Operation started: %s\n", operationID)

		if !dockerFollow {
			return nil
		}

		return streamOperation(cfg, serverID, stackName, operationID)
	}
}

func startOperation(cfg *config.Config, serverID, stackName string, req operationRequest) (string, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/servers/%s/stacks/%s/operations", cfg.Server, serverID, stackName)
	httpReq, err := http.NewRequest("POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	if cfg.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("operation failed: %s - %s", resp.Status, string(body))
	}

	var opResp operationResponse
	if err := json.NewDecoder(resp.Body).Decode(&opResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return opResp.OperationID, nil
}

func streamOperation(cfg *config.Config, serverID, stackName, operationID string) error {
	wsPath := fmt.Sprintf("/ws/api/servers/%s/stacks/%s/operations/%s", serverID, stackName, operationID)
	conn, err := ws.Dial(cfg, wsPath)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	defer conn.Close()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return fmt.Errorf("WebSocket read error: %w", err)
		}

		var msg streamMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing message: %v\n", err)
			continue
		}

		switch msg.Type {
		case "stdout", "stderr":
			if len(msg.Data) > 0 && msg.Data[len(msg.Data)-1] != '\n' {
				fmt.Println(msg.Data)
			} else {
				fmt.Print(msg.Data)
			}
		case "progress":
			fmt.Printf("[%s] %s\n", msg.Timestamp.Format("15:04:05"), msg.Data)
		case "complete":
			exitCode := 0
			if msg.ExitCode != nil {
				exitCode = *msg.ExitCode
			}
			success := msg.Success != nil && *msg.Success
			if success {
				fmt.Printf("\nOperation completed successfully (exit code: %d)\n", exitCode)
			} else {
				fmt.Printf("\nOperation failed (exit code: %d)\n", exitCode)
			}
			os.Exit(exitCode)
		case "error":
			fmt.Fprintf(os.Stderr, "Error: %s\n", msg.Data)
			os.Exit(1)
		}
	}
}
