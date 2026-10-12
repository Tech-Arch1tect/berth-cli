package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Tech-Arch1tect/berth-cli/pkg/output"
	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var inspectCmd = &cobra.Command{
	Use:         "inspect [service]",
	Short:       "Inspect stack services and runtime containers",
	Long:        "Inspect a stack's runtime details, optionally selecting a service or exact API container name.",
	Args:        cobra.RangeArgs(0, 1),
	RunE:        runInspect,
	Annotations: mcpMetadata("read", "service?"),
}

func init() {
	rootCmd.AddCommand(inspectCmd)
	addServerIDFlag(inspectCmd, false)
	addStackFlag(inspectCmd, false)
	inspectCmd.Flags().String("container", "", "Select an exact API container name")
}

func runInspect(cmd *cobra.Command, args []string) error {
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
	format, err := getOutputFormat()
	if err != nil {
		return err
	}
	c, err := newClient(cmd)
	if err != nil {
		return err
	}
	resp, _, err := c.API.StacksAPI.ApiV1ServersServeridStacksStacknameGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to inspect stack: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to inspect stack: API returned success=false")
	}
	data := resp.GetData()
	if serviceName != "" {
		found := false
		for i := range data.Services {
			if data.Services[i].Name == serviceName {
				data.Services = data.Services[i : i+1]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("service %q not found in stack %q", serviceName, stackName)
		}
	}
	if containerName != "" {
		found := false
		for i := range data.Services {
			for j := range data.Services[i].Containers {
				if data.Services[i].Containers[j].Name == containerName {
					data.Services[i].Containers = data.Services[i].Containers[j : j+1]
					data.Services = data.Services[i : i+1]
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return fmt.Errorf("container %q not found in selected services of stack %q", containerName, stackName)
		}
	}
	printer := output.NewWithColour(format, output.ColourSupported(flagNoColour))
	if format == output.FormatJSON {
		return printer.Print(os.Stdout, data)
	}
	table := &output.TableData{
		Columns: []output.Column{
			{Header: "Service", Field: "service"},
			{Header: "Container", Field: "container"},
			{Header: "Image", Field: "image"},
			{Header: "State", Field: "state"},
			{Header: "Health", Field: "health"},
			{Header: "Exit Code", Field: "exit_code"},
			{Header: "Ports", Field: "ports"},
		},
	}
	for _, service := range data.Services {
		if len(service.Containers) == 0 {
			table.Rows = append(table.Rows, map[string]any{
				"service": service.Name, "container": "n/a", "image": "n/a",
				"state": "n/a", "health": "n/a", "exit_code": "n/a", "ports": "n/a",
			})
		}
		for _, container := range service.Containers {
			var exitCode any = "n/a"
			if value, ok := container.GetExitCodeOk(); ok && value != nil {
				exitCode = *value
			}
			health := "n/a"
			if value, ok := container.GetHealthOk(); ok && value != nil && value.Status != "" {
				health = value.Status
			}
			table.Rows = append(table.Rows, map[string]any{
				"service": service.Name, "container": container.Name, "image": container.Image,
				"state": container.State, "health": health, "exit_code": exitCode,
				"ports": inspectPorts(container.Ports),
			})
		}
	}
	if len(table.Rows) == 0 {
		return printer.PrintEmpty(os.Stdout, "No services found.")
	}
	return printer.Print(os.Stdout, table)
}

func inspectPorts(ports []berth.Port) string {
	if len(ports) == 0 {
		return "n/a"
	}
	var bindings strings.Builder
	for i, port := range ports {
		if i > 0 {
			bindings.WriteString(", ")
		}
		if port.Public != nil && *port.Public != 0 {
			fmt.Fprintf(&bindings, "%d:%d/%s", *port.Public, port.Private, port.Type)
		} else {
			fmt.Fprintf(&bindings, "%d/%s", port.Private, port.Type)
		}
	}
	return bindings.String()
}
