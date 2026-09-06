package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	composeGetOutputFormat string
	composeGetFieldFilter  string
)

var composeGetCmd = &cobra.Command{
	Use:   "get [service]",
	Short: "Get compose configuration",
	Long: `Retrieve and display the compose configuration for a stack.

Examples:
  # Get full stack config as YAML
  berth-cli compose get -s 1 -n my-stack

  # Get specific service config
  berth-cli compose get -s 1 -n my-stack nginx

  # Output as JSON
  berth-cli compose get -s 1 -n my-stack --output json

  # Get specific field from a service
  berth-cli compose get -s 1 -n my-stack nginx --field image
  berth-cli compose get -s 1 -n my-stack nginx --field environment
  berth-cli compose get -s 1 -n my-stack nginx --field ports`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runComposeGet,
}

func init() {
	composeCmd.AddCommand(composeGetCmd)
	composeGetCmd.Flags().StringVarP(&composeGetOutputFormat, "output", "o", "yaml", "Output format: yaml, json")
	composeGetCmd.Flags().StringVarP(&composeGetFieldFilter, "field", "f", "", "Extract specific field (e.g., image, ports, environment)")
}

func runComposeGet(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	var serviceName string
	if len(args) > 0 {
		serviceName = args[0]
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	data := resp.GetData()

	config := make(map[string]any)
	config["services"] = data.GetServices()
	if data.HasNetworks() {
		config["networks"] = data.GetNetworks()
	}
	if data.HasVolumes() {
		config["volumes"] = data.GetVolumes()
	}
	if data.HasConfigs() {
		config["configs"] = data.GetConfigs()
	}
	if data.HasSecrets() {
		config["secrets"] = data.GetSecrets()
	}

	var output any = config

	if serviceName != "" {
		services := data.GetServices()
		service, ok := services[serviceName]
		if !ok {
			return fmt.Errorf("service '%s' not found", serviceName)
		}

		output = service

		if composeGetFieldFilter != "" {
			fieldValue, ok := service[composeGetFieldFilter]
			if !ok {
				return fmt.Errorf("field '%s' not found in service '%s'", composeGetFieldFilter, serviceName)
			}
			output = fieldValue
		}
	} else if composeGetFieldFilter != "" {
		return fmt.Errorf("--field requires a service name")
	}

	switch composeGetOutputFormat {
	case "json":
		formatted, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format JSON: %w", err)
		}
		fmt.Println(string(formatted))

	case "yaml":
		formatted, err := yaml.Marshal(output)
		if err != nil {
			return fmt.Errorf("failed to format YAML: %w", err)
		}
		fmt.Print(string(formatted))

	default:
		return fmt.Errorf("unknown output format: %s (use 'yaml' or 'json')", composeGetOutputFormat)
	}

	return nil
}
