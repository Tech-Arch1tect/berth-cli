package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var composeAddNetworkCmd = &cobra.Command{
	Use:   "add-network <server-id> <stack-name> <service> <network-name>",
	Short: "Add a service to a network",
	Long: `Add a service to a network in a Docker Compose stack.

The network must already exist at the stack level.

Examples:
  # Add service to a network
  berth-cli compose add-network 1 my-stack nginx frontend

  # Skip confirmation
  berth-cli compose add-network 1 my-stack nginx backend --yes`,
	Args: cobra.ExactArgs(4),
	RunE: runComposeAddNetwork,
}

var composeRemoveNetworkCmd = &cobra.Command{
	Use:   "remove-network <server-id> <stack-name> <service> <network-name>",
	Short: "Remove a service from a network",
	Long: `Remove a service from a network in a Docker Compose stack.

Examples:
  # Remove service from a network
  berth-cli compose remove-network 1 my-stack nginx frontend

  # Skip confirmation
  berth-cli compose remove-network 1 my-stack nginx frontend --yes`,
	Args: cobra.ExactArgs(4),
	RunE: runComposeRemoveNetwork,
}

func init() {
	composeCmd.AddCommand(composeAddNetworkCmd)
	composeCmd.AddCommand(composeRemoveNetworkCmd)
	composeAddNetworkCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeRemoveNetworkCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func getServiceNetworks(services map[string]map[string]any, serviceName string) (map[string]any, error) {
	service, ok := services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service '%s' not found", serviceName)
	}

	networksRaw, ok := service["networks"]
	if !ok || networksRaw == nil {
		return map[string]any{}, nil
	}

	networks, ok := networksRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid networks format")
	}

	result := make(map[string]any)
	for name, cfg := range networks {
		if cfg == nil {
			result[name] = map[string]any{}
		} else {
			result[name] = cfg
		}
	}

	return result, nil
}

func runComposeAddNetwork(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	var serverID int32
	if _, err := fmt.Sscanf(args[0], "%d", &serverID); err != nil {
		return fmt.Errorf("invalid server ID: %s", args[0])
	}

	stackName := args[1]
	serviceName := args[2]
	networkName := args[3]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	stackNetworks := resp.GetNetworks()
	if _, exists := stackNetworks[networkName]; !exists {
		return fmt.Errorf("network '%s' does not exist in stack. Define it first with 'compose create-network'", networkName)
	}

	currentNetworks, err := getServiceNetworks(resp.GetServices(), serviceName)
	if err != nil {
		return err
	}

	if _, exists := currentNetworks[networkName]; exists {
		return fmt.Errorf("service '%s' is already in network '%s'", serviceName, networkName)
	}

	currentNetworks[networkName] = map[string]any{}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"networks": currentNetworks,
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully added service '%s' to network '%s'\n", serviceName, networkName)
	return nil
}

func runComposeRemoveNetwork(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	var serverID int32
	if _, err := fmt.Sscanf(args[0], "%d", &serverID); err != nil {
		return fmt.Errorf("invalid server ID: %s", args[0])
	}

	stackName := args[1]
	serviceName := args[2]
	networkName := args[3]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	currentNetworks, err := getServiceNetworks(resp.GetServices(), serviceName)
	if err != nil {
		return err
	}

	if _, exists := currentNetworks[networkName]; !exists {
		return fmt.Errorf("service '%s' is not in network '%s'", serviceName, networkName)
	}

	if len(currentNetworks) == 1 && networkName == "default" {
		return fmt.Errorf("cannot remove service from 'default' network when it's the only network.\nDocker Compose requires services to be on at least one network")
	}

	if len(currentNetworks) == 1 {
		fmt.Printf("Warning: Removing the last network from service '%s'.\n", serviceName)
		fmt.Println("Docker Compose will automatically add the service to the default network.")
	}

	delete(currentNetworks, networkName)

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"networks": currentNetworks,
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully removed service '%s' from network '%s'\n", serviceName, networkName)
	return nil
}
