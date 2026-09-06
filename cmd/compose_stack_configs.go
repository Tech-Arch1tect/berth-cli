package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var composeCreateConfigCmd = &cobra.Command{
	Use:   "create-config <config-name>",
	Short: "Create a config in a stack",
	Long: `Create a new config definition in a Docker Compose stack.

Configs can be sourced from a file or environment variable.

Examples:
  # Create config from file
  berth-cli compose create-config -s 1 -n my-stack nginx_config --file ./configs/nginx.conf

  # Create config from environment variable
  berth-cli compose create-config -s 1 -n my-stack app_config --environment APP_CONFIG

  # Create an external config reference
  berth-cli compose create-config -s 1 -n my-stack existing-config --external

  # Skip confirmation
  berth-cli compose create-config -s 1 -n my-stack settings --file ./config.json --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runComposeCreateConfig,
}

var composeDeleteConfigCmd = &cobra.Command{
	Use:   "delete-config <config-name>",
	Short: "Delete a config from a stack",
	Long: `Delete a config definition from a Docker Compose stack.

Examples:
  # Delete a config
  berth-cli compose delete-config -s 1 -n my-stack old-config

  # Skip confirmation
  berth-cli compose delete-config -s 1 -n my-stack unused-config --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runComposeDeleteConfig,
}

func init() {
	composeCmd.AddCommand(composeCreateConfigCmd)
	composeCmd.AddCommand(composeDeleteConfigCmd)

	composeCreateConfigCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeCreateConfigCmd.Flags().String("file", "", "Path to config file")
	composeCreateConfigCmd.Flags().String("environment", "", "Environment variable containing the config")
	composeCreateConfigCmd.Flags().Bool("external", false, "Mark as external config")

	composeDeleteConfigCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func stackConfigExists(configs map[string]map[string]any, configName string) bool {
	_, exists := configs[configName]
	return exists
}

func runComposeCreateConfig(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	configName := args[0]
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	file, _ := cmd.Flags().GetString("file")
	environment, _ := cmd.Flags().GetString("environment")
	external, _ := cmd.Flags().GetBool("external")

	if file == "" && environment == "" && !external {
		return fmt.Errorf("must specify --file, --environment, or --external")
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	data := resp.GetData()

	if stackConfigExists(data.GetConfigs(), configName) {
		return fmt.Errorf("config '%s' already exists in stack", configName)
	}

	cfgConfig := map[string]any{}
	if file != "" {
		cfgConfig["file"] = file
	}
	if environment != "" {
		cfgConfig["environment"] = environment
	}
	if external {
		cfgConfig["external"] = true
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"config_changes": map[string]any{
			configName: cfgConfig,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully created config '%s'\n", configName)
	return nil
}

func runComposeDeleteConfig(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	configName := args[0]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	data := resp.GetData()

	if !stackConfigExists(data.GetConfigs(), configName) {
		return fmt.Errorf("config '%s' not found in stack", configName)
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"config_changes": map[string]any{
			configName: nil,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted config '%s'\n", configName)
	return nil
}
