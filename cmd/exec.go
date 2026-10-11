package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	"github.com/Tech-Arch1tect/berth-cli/pkg/ws"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

const execLivenessWait = 90 * time.Second

var execCmd = &cobra.Command{
	Use:   "exec SERVICE -- COMMAND [ARG...]",
	Short: "Run a command in a stack service container",
	Long: `Run explicit command arguments in a service container using the terminal PTY.

Output is one combined terminal stream, written unchanged to stdout. Local stdin
is not forwarded. Commands requiring input may wait until interrupted. Output
format and colour flags do not change the terminal bytes.

Examples:
  berth-cli exec -s 1 -n my-stack web -- ls -al
  berth-cli exec -s 1 -n my-stack web --container my-stack-web-2 -- ls -al
  berth-cli exec -s 1 -n my-stack web -- sh -c 'ls -al /tmp'`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 || cmd.ArgsLenAtDash() != 1 || args[0] == "" || args[1] == "" {
			return fmt.Errorf("use exec -s SERVER -n STACK SERVICE -- COMMAND [ARG...]")
		}
		return nil
	},
	RunE:        runExec,
	Annotations: mcpMetadata("exec", "service", "command..."),
}

func init() {
	addServerIDFlag(execCmd, false)
	addStackFlag(execCmd, false)
	execCmd.Flags().String("container", "", "Exact container name; defaults to the first running service replica")
	execCmd.Flags().Duration("startup-timeout", 30*time.Second, "Maximum time to connect and receive session acknowledgement")
	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	startupTimeout, _ := cmd.Flags().GetDuration("startup-timeout")
	if startupTimeout <= 0 {
		return fmt.Errorf("--startup-timeout must be positive")
	}
	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	start := berth.NewTerminalStartMessage(args[0], "terminal_start")
	start.SetStackName(getStackName(cmd))
	start.SetCommand(args[1:])
	start.SetCols(80)
	start.SetRows(24)
	containerName, _ := cmd.Flags().GetString("container")
	if containerName != "" {
		start.SetContainerName(containerName)
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	return streamExec(ctx, cfg, serverID, *start, startupTimeout, cmd.OutOrStdout())
}

type CommandExitError struct {
	Code        int
	Interrupted bool
}

func (e *CommandExitError) Error() string {
	if e.Interrupted {
		return "command interrupted; remote execution may continue"
	}
	return fmt.Sprintf("command exited with code %d", e.Code)
}

func streamExec(ctx context.Context, cfg *config.Config, serverID int32, start berth.TerminalStartMessage, startupTimeout time.Duration, output io.Writer) (err error) {
	defer func() {
		if ctx.Err() != nil {
			err = &CommandExitError{Code: 130, Interrupted: true}
		}
	}()
	startupCtx, cancelStartup := context.WithTimeout(ctx, startupTimeout)
	defer cancelStartup()
	path := fmt.Sprintf("/ws/api/servers/%d/stacks/%s/terminal", serverID, start.GetStackName())
	conn, err := ws.Dial(startupCtx, cfg, path)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
	stopStartup := context.AfterFunc(startupCtx, func() { _ = conn.Close() })
	defer stopStartup()
	deadline, _ := startupCtx.Deadline()
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	conn.SetReadLimit(1 << 20)
	if err := conn.WriteJSON(start); err != nil {
		return fmt.Errorf("command start was not confirmed; remote execution may continue: %w", err)
	}
	sessionID := ""
	pingHandler := conn.PingHandler()
	conn.SetPingHandler(func(data string) error {
		if err := pingHandler(data); err != nil {
			return err
		}
		if sessionID != "" {
			return conn.SetReadDeadline(time.Now().Add(execLivenessWait))
		}
		return nil
	})

	fields := make(map[string]json.RawMessage)
	var decoded []byte
	for {
		frameType, data, err := conn.ReadMessage()
		if err != nil {
			if sessionID == "" {
				return fmt.Errorf("command startup was not confirmed; remote execution may continue: %w", err)
			}
			return fmt.Errorf("command has no measured result; remote execution may continue: %w", err)
		}
		if frameType != websocket.TextMessage {
			return fmt.Errorf("terminal sent a non-text message")
		}
		if err := terminalFrameFields(data, fields); err != nil {
			return fmt.Errorf("invalid terminal message: %w", err)
		}
		var messageType string
		if err := json.Unmarshal(fields["type"], &messageType); err != nil {
			return fmt.Errorf("invalid terminal message type: %w", err)
		}
		switch messageType {
		case "success":
			var message berth.TerminalSuccessMessage
			if err := json.Unmarshal(data, &message); err != nil {
				return fmt.Errorf("invalid terminal acknowledgement: %w", err)
			}
			if sessionID != "" || message.SessionId == "" || startupCtx.Err() != nil || !stopStartup() {
				return fmt.Errorf("invalid or late terminal acknowledgement")
			}
			sessionID = message.SessionId
			cancelStartup()
		case "terminal_output":
			var message berth.TerminalOutputMessage
			if err := json.Unmarshal(data, &message); err != nil {
				return fmt.Errorf("invalid terminal output: %w", err)
			}
			if sessionID == "" || message.SessionId != sessionID || bytes.Equal(bytes.TrimSpace(fields["output"]), []byte("null")) {
				return fmt.Errorf("invalid terminal output session or bytes")
			}
			decoded, err = base64.StdEncoding.AppendDecode(decoded[:0], []byte(message.Output))
			if err != nil {
				return fmt.Errorf("invalid terminal output encoding: %w", err)
			}
			if len(decoded) > 0 {
				n, err := output.Write(decoded)
				if err != nil {
					return fmt.Errorf("failed to write command output; remote execution may continue: %w", err)
				}
				if n != len(decoded) {
					return fmt.Errorf("failed to write command output: %w", io.ErrShortWrite)
				}
			}
		case "terminal_close":
			var message berth.TerminalCloseMessage
			if err := json.Unmarshal(data, &message); err != nil {
				return fmt.Errorf("invalid terminal result: %w", err)
			}
			exitCode := message.ExitCode.Get()
			if sessionID == "" || message.SessionId != sessionID || exitCode == nil || *exitCode < 0 || *exitCode > 255 {
				return fmt.Errorf("terminal closed without a valid measured command result")
			}
			if *exitCode != 0 {
				return &CommandExitError{Code: int(*exitCode)}
			}
			return nil
		case "error":
			var message string
			if err := json.Unmarshal(fields["error"], &message); err != nil || message == "" {
				return fmt.Errorf("terminal reported an invalid error")
			}
			return fmt.Errorf("terminal rejected the command: %s", message)
		default:
			return fmt.Errorf("terminal sent unknown message type %q", messageType)
		}
		if err := conn.SetReadDeadline(time.Now().Add(execLivenessWait)); err != nil {
			return err
		}
	}
}

func terminalFrameFields(data []byte, fields map[string]json.RawMessage) error {
	clear(fields)
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("expected a JSON object")
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return fmt.Errorf("invalid field name")
		}
		switch name {
		case "type", "timestamp", "session_id", "message", "output", "exit_code", "error", "context":
		default:
			return fmt.Errorf("unknown terminal field %q", name)
		}
		if _, exists := fields[name]; exists {
			return fmt.Errorf("duplicate terminal field %q", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("unexpected data after terminal message")
	}
	return nil
}
