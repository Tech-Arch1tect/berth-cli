package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/Tech-Arch1tect/berth-cli/pkg/output"
	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var statsCmd = &cobra.Command{
	Use:         "stats [service]",
	Short:       "Read one stack resource statistics snapshot",
	Long:        "Read one API-reported stack statistics snapshot, optionally selecting a service or exact API container name.",
	Args:        cobra.RangeArgs(0, 1),
	RunE:        runStats,
	Annotations: mcpMetadata("read", "service?"),
}

func init() {
	rootCmd.AddCommand(statsCmd)
	addServerIDFlag(statsCmd, false)
	addStackFlag(statsCmd, false)
	statsCmd.Flags().String("container", "", "Select an exact API container name")
}

func runStats(cmd *cobra.Command, args []string) error {
	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}
	stackName := getStackName(cmd)
	if stackName == "" {
		return fmt.Errorf("stack name must not be empty")
	}
	serviceName := ""
	if len(args) > 0 {
		serviceName = args[0]
		if serviceName == "" {
			return fmt.Errorf("service name must not be empty")
		}
	}
	containerName, _ := cmd.Flags().GetString("container")
	if cmd.Flags().Changed("container") && containerName == "" {
		return fmt.Errorf("container name must not be empty")
	}
	if flagOutput == "" {
		return fmt.Errorf("output format must not be empty")
	}
	format, err := getOutputFormat()
	if err != nil {
		return err
	}
	c, err := newClient(cmd)
	if err != nil {
		return err
	}
	resp, _, err := c.API.StacksAPI.ApiV1ServersServeridStacksStacknameStatsGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to read stack stats: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("failed to read stack stats: API returned no response")
	}
	if !resp.Success {
		return fmt.Errorf("failed to read stack stats: API returned success=false")
	}
	data := resp.GetData()
	if serviceName != "" {
		selected := data.Containers[:0]
		for i := range data.Containers {
			if data.Containers[i].ServiceName == serviceName {
				selected = append(selected, data.Containers[i])
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("service %q not found in stack %q", serviceName, stackName)
		}
		data.Containers = selected
	}
	if containerName != "" {
		found := false
		for i := range data.Containers {
			if data.Containers[i].Name == containerName {
				data.Containers = data.Containers[i : i+1]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("container %q not found in selected services of stack %q", containerName, stackName)
		}
	}
	if data.Containers == nil {
		data.Containers = []berth.ContainerStats{}
	}
	printer := output.NewWithColour(format, output.ColourSupported(flagNoColour))
	if format == output.FormatJSON {
		return printer.Print(os.Stdout, data)
	}
	window := "n/a"
	if value := data.SampleWindowSeconds.Get(); value != nil {
		window = fmt.Sprintf("%.3g s", *value)
	}
	if _, err := fmt.Fprintf(os.Stdout, "Stack: %s\nCollected at: %s\nSample window: %s\n\nHOST context\nCPU cores: %d\nMemory available: %d B\nMemory total: %d B\nLoad (1/5/15 min): %.3g / %.3g / %.3g\n\nCONTAINERS in snapshot\n", data.StackName, data.CollectedAt.Format(time.RFC3339Nano), window, data.Host.CpuCores, data.Host.MemoryAvailable, data.Host.MemoryTotal, data.Host.Load1, data.Host.Load5, data.Host.Load15); err != nil {
		return err
	}
	table := &output.TableData{
		Columns: []output.Column{
			{Header: "Service", Field: "service"},
			{Header: "Container", Field: "container"},
			{Header: "State", Field: "state"},
			{Header: "CPU cores", Field: "cpu"},
			{Header: "Memory working set", Field: "memory"},
			{Header: "Memory limit", Field: "limit"},
			{Header: "Network Rx B/s", Field: "rx"},
			{Header: "Network Tx B/s", Field: "tx"},
			{Header: "Block read B/s", Field: "read"},
			{Header: "Block write B/s", Field: "write"},
		},
		Rows: make([]map[string]any, 0, len(data.Containers)),
	}
	for i := range data.Containers {
		container := &data.Containers[i]
		row := map[string]any{
			"service": container.ServiceName, "container": container.Name, "state": container.State,
			"cpu": "n/a", "memory": "n/a", "limit": "n/a",
			"rx": "n/a", "tx": "n/a", "read": "n/a", "write": "n/a",
		}
		if container.State == "running" {
			row["cpu"] = statsFloat(container.CpuUsageCores)
			row["memory"] = fmt.Sprintf("%d B", container.MemoryWorkingSet)
			if container.MemoryLimit != 0 {
				row["limit"] = fmt.Sprintf("%d B", container.MemoryLimit)
			}
			row["rx"] = statsFloat(container.NetworkRxBytesPerSecond)
			row["tx"] = statsFloat(container.NetworkTxBytesPerSecond)
			row["read"] = statsFloat(container.BlockReadBytesPerSecond)
			row["write"] = statsFloat(container.BlockWriteBytesPerSecond)
		}
		table.Rows = append(table.Rows, row)
	}
	if len(table.Rows) == 0 {
		return printer.PrintEmpty(os.Stdout, "No containers in snapshot.")
	}
	return printer.Print(os.Stdout, table)
}

func statsFloat(value berth.NullableFloat32) string {
	if number := value.Get(); number != nil {
		return fmt.Sprintf("%.3g", *number)
	}
	return "n/a"
}
