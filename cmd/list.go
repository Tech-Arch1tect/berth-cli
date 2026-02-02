package cmd

import (
	"fmt"
	"os"

	"github.com/Tech-Arch1tect/berth-cli/pkg/output"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources",
	Long:  `List servers and stacks.`,
}

var listServersCmd = &cobra.Command{
	Use:   "servers",
	Short: "List all servers",
	Long:  `List all accessible Berth servers.`,
	RunE:  runListServers,
}

var listStacksCmd = &cobra.Command{
	Use:   "stacks <server-id>",
	Short: "List stacks on a server",
	Long:  `List all stacks on a specific server.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runListStacks,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listServersCmd)
	listCmd.AddCommand(listStacksCmd)
}

func runListServers(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	format, err := getOutputFormat()
	if err != nil {
		return err
	}

	resp, _, err := c.API.ServersAPI.ApiV1ServersGet(c.Ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	data := resp.GetData()
	servers := data.GetServers()
	if len(servers) == 0 {
		fmt.Println("No servers found.")
		return nil
	}

	tableData := &output.TableData{
		Columns: []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "Name", Field: "name"},
			{Header: "Host", Field: "host"},
			{Header: "Port", Field: "port"},
			{Header: "Active", Field: "active"},
			{Header: "SSL Skip", Field: "ssl_skip"},
		},
		Rows: make([]map[string]any, 0, len(servers)),
	}

	for _, server := range servers {
		tableData.Rows = append(tableData.Rows, map[string]any{
			"id":       server.GetId(),
			"name":     server.GetName(),
			"host":     server.GetHost(),
			"port":     server.GetPort(),
			"active":   server.GetIsActive(),
			"ssl_skip": server.GetSkipSslVerification(),
		})
	}

	return output.New(format).Print(os.Stdout, tableData)
}

func runListStacks(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	format, err := getOutputFormat()
	if err != nil {
		return err
	}

	serverID := args[0]

	var serverIDInt int32
	if _, err := fmt.Sscanf(serverID, "%d", &serverIDInt); err != nil {
		return fmt.Errorf("invalid server ID: %s", serverID)
	}

	resp, _, err := c.API.StacksAPI.ApiV1ServersServeridStacksGet(c.Ctx, serverIDInt).Execute()
	if err != nil {
		return fmt.Errorf("failed to list stacks: %w", err)
	}

	data := resp.GetData()
	stacks := data.GetStacks()
	if len(stacks) == 0 {
		fmt.Printf("No stacks found on server %s.\n", serverID)
		return nil
	}

	tableData := &output.TableData{
		Columns: []output.Column{
			{Header: "Name", Field: "name"},
			{Header: "Healthy", Field: "healthy"},
			{Header: "Total", Field: "total"},
			{Header: "Running", Field: "running"},
			{Header: "Compose File", Field: "compose_file"},
		},
		Rows: make([]map[string]any, 0, len(stacks)),
	}

	for _, stack := range stacks {
		tableData.Rows = append(tableData.Rows, map[string]any{
			"name":         stack.GetName(),
			"healthy":      stack.GetIsHealthy(),
			"total":        stack.GetTotalContainers(),
			"running":      stack.GetRunningContainers(),
			"compose_file": stack.GetComposeFile(),
		})
	}

	return output.New(format).Print(os.Stdout, tableData)
}
