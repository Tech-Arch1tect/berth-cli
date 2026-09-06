package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Tech-Arch1tect/berth-cli/pkg/client"
	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	"github.com/Tech-Arch1tect/berth-cli/pkg/ws"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
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

func runDockerOperation(command string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(cmd)
		if err != nil {
			return err
		}

		serverID, err := getServerID(cmd)
		if err != nil {
			return err
		}

		operationID, err := startOperation(client.New(cfg), serverID, getStackName(cmd), berth.OperationRequest{
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

		return streamOperation(cfg, getServerIDStr(cmd), getStackName(cmd), operationID)
	}
}

func startOperation(c *client.Client, serverID int32, stackName string, req berth.OperationRequest) (string, error) {
	resp, _, err := c.API.OperationsAPI.ApiV1ServersServeridStacksStacknameOperationsPost(c.Ctx, serverID, stackName).
		OperationRequest(req).Execute()
	if err != nil {
		return "", fmt.Errorf("failed to start operation: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("operation start rejected")
	}
	if resp.Data.OperationId == "" {
		return "", fmt.Errorf("response contained no operation ID")
	}
	return resp.Data.OperationId, nil
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

		var msg berth.StreamMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing message: %v\n", err)
			continue
		}

		switch msg.Type {
		case "stdout", "stderr":
			if msg.Data == nil {
				continue
			}
			data := *msg.Data
			if len(data) > 0 && data[len(data)-1] != '\n' {
				fmt.Println(data)
			} else {
				fmt.Print(data)
			}
		case "progress":
			if msg.Data != nil {
				fmt.Printf("[%s] %s\n", msg.Timestamp.Format("15:04:05"), *msg.Data)
			}
		case "complete":
			exitCode := 0
			if msg.ExitCode.IsSet() && msg.ExitCode.Get() != nil {
				exitCode = int(*msg.ExitCode.Get())
			}
			success := msg.Success.IsSet() && msg.Success.Get() != nil && *msg.Success.Get()
			if success {
				fmt.Printf("\nOperation completed successfully (exit code: %d)\n", exitCode)
			} else {
				fmt.Printf("\nOperation failed (exit code: %d)\n", exitCode)
			}
			os.Exit(exitCode)
		case "error":
			if msg.Data != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", *msg.Data)
			}
			os.Exit(1)
		}
	}
}
