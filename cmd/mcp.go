package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:         "mcp",
	Short:       "Run the stdio MCP server",
	Long:        "Serve management tools over stdio using BERTH_SERVER_URL, BERTH_API_KEY and BERTH_INSECURE. Compose mutations preview changes unless yes is true. Cancelling a tool stops the local CLI process; remote operations may continue.",
	Args:        cobra.NoArgs,
	Annotations: mcpTransportMetadata(),
	RunE:        runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, _ []string) error {
	for _, name := range []string{"server", "api-key", "insecure", "verbose"} {
		if cmd.Flags().Changed(name) || cmd.InheritedFlags().Changed(name) || cmd.Root().PersistentFlags().Changed(name) {
			return fmt.Errorf("--%s is not supported in MCP mode; configure the connection through environment variables", name)
		}
	}
	commands, err := collectMCPCommands(cmd.Root())
	if err != nil {
		return err
	}
	cfg, err := config.LoadMCP()
	if err != nil {
		return err
	}
	executor, err := newMCPExecutor(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	server := mcp.NewServer(&mcp.Implementation{Name: "berth-cli", Version: Version}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{},
		Instructions: "Tools run isolated Berth CLI processes with a connection fixed when this server starts. Compose mutation tools preview by default and apply only when yes is true. stdout and stderr capture at most 1 MiB each of original output and redact the API key before encoding; binary output is base64. exit_code measures the local CLI process, not remote termination. Cancellation stops the local process and remote operations may continue.",
	})
	if err := registerMCPTools(server, commands, executor, lifetime); err != nil {
		return err
	}
	return server.Run(ctx, &mcpLifecycleTransport{cancel: cancel})
}

func registerMCPTools(server *mcp.Server, commands []mcpCommand, executor *mcpExecutor, lifetime context.Context) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("registering MCP tools: %s", executor.redact(fmt.Sprint(failure)))
		}
	}()
	for _, command := range commands {
		mcp.AddTool(server, command.tool, func(ctx context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, mcpExecutionResult, error) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			stop := context.AfterFunc(lifetime, cancel)
			defer stop()
			if lifetime.Err() != nil {
				cancel()
			}
			output, err := executor.execute(ctx, command, input)
			if err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: executor.redact(err.Error())}},
				}, output, nil
			}
			return nil, output, nil
		})
	}
	return nil
}

type mcpLifecycleTransport struct {
	cancel context.CancelFunc
}

func (t *mcpLifecycleTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := (&mcp.StdioTransport{}).Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &mcpLifecycleConnection{Connection: connection, cancel: t.cancel}, nil
}

type mcpLifecycleConnection struct {
	mcp.Connection
	cancel context.CancelFunc
}

func (c *mcpLifecycleConnection) Close() error {
	c.cancel()
	return c.Connection.Close()
}
