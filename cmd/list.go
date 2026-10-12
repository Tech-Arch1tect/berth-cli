package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Tech-Arch1tect/berth-cli/pkg/output"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources",
	Long:  `List servers and stacks.`,
}

var listServersCmd = &cobra.Command{
	Use:         "servers",
	Short:       "List all servers",
	Long:        `List all accessible Berth servers.`,
	RunE:        runListServers,
	Annotations: mcpMetadata("read"),
}

var listStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List and search stacks",
	Long: `List stacks on a selected server or all accessible servers.

Use --server-id or --all. Search matches stack name, server name, Compose
filename and path without reading configuration contents. Health filtering
uses the API's aggregate stack health flag.

Results include server IDs and names. Failed or inactive servers make an
all-server search incomplete: available matches are returned, diagnostics
are written to stderr, and the command exits non-zero.`,
	RunE:        runListStacks,
	Annotations: mcpMetadata("read"),
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listServersCmd)
	listCmd.AddCommand(listStacksCmd)
	listStacksCmd.Flags().StringP("server-id", "s", "", "Server ID (required unless --all is set)")
	listStacksCmd.Flags().Bool("all", false, "Search all accessible servers")
	listStacksCmd.Flags().String("search", "", "Case-insensitive search of stack name, server name, Compose filename and path")
	listStacksCmd.Flags().String("health", "all", "Filter by stack health: all, healthy, unhealthy")
	listStacksCmd.MarkFlagsOneRequired("server-id", "all")
	listStacksCmd.MarkFlagsMutuallyExclusive("server-id", "all")
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
	printer := output.NewWithColour(format, output.ColourSupported(flagNoColour))

	if len(servers) == 0 {
		return printer.PrintEmpty(os.Stdout, "No servers found.")
	}

	tableData := &output.TableData{
		Columns: []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "Name", Field: "name"},
			{Header: "Host", Field: "host"},
			{Header: "Port", Field: "port"},
			{Header: "Active", Field: "active", Colour: output.BoolColour},
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

	return printer.Print(os.Stdout, tableData)
}

func runListStacks(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	var serverID int32
	if !all {
		var err error
		serverID, err = getServerID(cmd)
		if err != nil {
			return err
		}
	}
	health, _ := cmd.Flags().GetString("health")
	switch health {
	case "all", "healthy", "unhealthy":
	default:
		return fmt.Errorf("invalid health filter %q (valid: all, healthy, unhealthy)", health)
	}
	search, _ := cmd.Flags().GetString("search")
	search = strings.ToLower(search)
	format, err := getOutputFormat()
	if err != nil {
		return err
	}
	c, err := newClient(cmd)
	if err != nil {
		return err
	}
	tableData := &output.TableData{
		Columns: []output.Column{
			{Header: "Server ID", Field: "server_id"},
			{Header: "Server", Field: "server_name"},
			{Header: "Name", Field: "name"},
			{Header: "Healthy", Field: "healthy", Colour: output.BoolColour},
			{Header: "Total", Field: "total"},
			{Header: "Running", Field: "running"},
			{Header: "Compose File", Field: "compose_file"},
		},
		Rows: make([]map[string]any, 0),
	}
	appendStacks := func(id int32) error {
		resp, _, err := c.API.StacksAPI.ApiV1ServersServeridStacksGet(c.Ctx, id).Execute()
		if err != nil {
			return err
		}
		data := resp.GetData()
		for _, stack := range data.GetStacks() {
			if health == "healthy" && !stack.GetIsHealthy() || health == "unhealthy" && stack.GetIsHealthy() {
				continue
			}
			if search != "" &&
				!strings.Contains(strings.ToLower(stack.GetName()), search) &&
				!strings.Contains(strings.ToLower(stack.GetServerName()), search) &&
				!strings.Contains(strings.ToLower(stack.GetComposeFile()), search) &&
				!strings.Contains(strings.ToLower(stack.GetPath()), search) {
				continue
			}
			tableData.Rows = append(tableData.Rows, map[string]any{
				"server_id":    stack.GetServerId(),
				"server_name":  stack.GetServerName(),
				"name":         stack.GetName(),
				"healthy":      stack.GetIsHealthy(),
				"total":        stack.GetTotalContainers(),
				"running":      stack.GetRunningContainers(),
				"compose_file": stack.GetComposeFile(),
				"path":         stack.GetPath(),
			})
		}
		return nil
	}
	var failures []error
	if all {
		resp, _, err := c.API.ServersAPI.ApiV1ServersGet(c.Ctx).Execute()
		if err != nil {
			return fmt.Errorf("failed to list servers: %w", err)
		}
		data := resp.GetData()
		for _, server := range data.GetServers() {
			if !server.GetIsActive() {
				failures = append(failures, fmt.Errorf("server %s (%d): inactive", server.GetName(), server.GetId()))
				continue
			}
			if err := appendStacks(server.GetId()); err != nil {
				failures = append(failures, fmt.Errorf("server %s (%d): %w", server.GetName(), server.GetId(), err))
			}
		}
	} else if err := appendStacks(serverID); err != nil {
		return fmt.Errorf("failed to list stacks: %w", err)
	}
	printer := output.NewWithColour(format, output.ColourSupported(flagNoColour))
	if len(tableData.Rows) == 0 {
		err = printer.PrintEmpty(os.Stdout, "No matching stacks returned.")
	} else {
		err = printer.Print(os.Stdout, tableData)
	}
	if err != nil {
		return err
	}
	if len(failures) != 0 {
		return fmt.Errorf("stack search incomplete: %w", errors.Join(failures...))
	}
	return nil
}
