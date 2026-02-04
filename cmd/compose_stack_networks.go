package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var composeCreateNetworkCmd = &cobra.Command{
	Use:   "create-network <server-id> <stack-name> <network-name>",
	Short: "Create a network in a stack",
	Long: `Create a new network definition in a Docker Compose stack.

The network is created with default settings. Use flags to customize.

Examples:
  # Create a basic network
  berth-cli compose create-network 1 my-stack frontend

  # Create with custom driver
  berth-cli compose create-network 1 my-stack backend --driver overlay

  # Create with IPAM configuration
  berth-cli compose create-network 1 my-stack internal --subnet 172.28.0.0/16 --gateway 172.28.0.1

  # Create an external network reference
  berth-cli compose create-network 1 my-stack shared-net --external

  # Skip confirmation
  berth-cli compose create-network 1 my-stack internal --yes`,
	Args: cobra.ExactArgs(3),
	RunE: runComposeCreateNetwork,
}

var composeDeleteNetworkCmd = &cobra.Command{
	Use:   "delete-network <server-id> <stack-name> <network-name>",
	Short: "Delete a network from a stack",
	Long: `Delete a network definition from a Docker Compose stack.

The network must not be in use by any services.

Examples:
  # Delete a network
  berth-cli compose delete-network 1 my-stack old-network

  # Skip confirmation
  berth-cli compose delete-network 1 my-stack unused-net --yes`,
	Args: cobra.ExactArgs(3),
	RunE: runComposeDeleteNetwork,
}

func init() {
	composeCmd.AddCommand(composeCreateNetworkCmd)
	composeCmd.AddCommand(composeDeleteNetworkCmd)

	composeCreateNetworkCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeCreateNetworkCmd.Flags().String("driver", "", "Network driver (e.g., bridge, overlay)")
	composeCreateNetworkCmd.Flags().Bool("external", false, "Mark as external network")
	composeCreateNetworkCmd.Flags().String("subnet", "", "Subnet in CIDR format (e.g., 172.28.0.0/16)")
	composeCreateNetworkCmd.Flags().String("gateway", "", "Gateway IP address")
	composeCreateNetworkCmd.Flags().String("ip-range", "", "IP range for allocation")
	composeCreateNetworkCmd.Flags().String("ipam-driver", "", "IPAM driver (default: default)")

	composeDeleteNetworkCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func stackNetworkExists(networks map[string]map[string]any, networkName string) bool {
	_, exists := networks[networkName]
	return exists
}

func getServicesUsingNetwork(services map[string]map[string]any, networkName string) []string {
	var using []string
	for serviceName, svc := range services {
		if networks, ok := svc["networks"].(map[string]any); ok {
			if _, exists := networks[networkName]; exists {
				using = append(using, serviceName)
			}
		}
	}
	return using
}

func runComposeCreateNetwork(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	var serverID int32
	if _, err := fmt.Sscanf(args[0], "%d", &serverID); err != nil {
		return fmt.Errorf("invalid server ID: %s", args[0])
	}

	stackName := args[1]
	networkName := args[2]
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	driver, _ := cmd.Flags().GetString("driver")
	external, _ := cmd.Flags().GetBool("external")
	subnet, _ := cmd.Flags().GetString("subnet")
	gateway, _ := cmd.Flags().GetString("gateway")
	ipRange, _ := cmd.Flags().GetString("ip-range")
	ipamDriver, _ := cmd.Flags().GetString("ipam-driver")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	if stackNetworkExists(resp.GetNetworks(), networkName) {
		return fmt.Errorf("network '%s' already exists in stack", networkName)
	}

	netConfig := map[string]any{}
	if driver != "" {
		netConfig["driver"] = driver
	}
	if external {
		netConfig["external"] = true
	}

	if subnet != "" || gateway != "" || ipRange != "" || ipamDriver != "" {
		ipamConfig := map[string]any{}
		if ipamDriver != "" {
			ipamConfig["driver"] = ipamDriver
		}
		if subnet != "" || gateway != "" || ipRange != "" {
			pool := map[string]any{}
			if subnet != "" {
				pool["subnet"] = subnet
			}
			if gateway != "" {
				pool["gateway"] = gateway
			}
			if ipRange != "" {
				pool["ip_range"] = ipRange
			}
			ipamConfig["config"] = []map[string]any{pool}
		}
		netConfig["ipam"] = ipamConfig
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"network_changes": map[string]any{
			networkName: netConfig,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully created network '%s'\n", networkName)
	return nil
}

func runComposeDeleteNetwork(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	var serverID int32
	if _, err := fmt.Sscanf(args[0], "%d", &serverID); err != nil {
		return fmt.Errorf("invalid server ID: %s", args[0])
	}

	stackName := args[1]
	networkName := args[2]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if networkName == "default" {
		return fmt.Errorf("cannot delete the default network")
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	if !stackNetworkExists(resp.GetNetworks(), networkName) {
		return fmt.Errorf("network '%s' not found in stack", networkName)
	}

	using := getServicesUsingNetwork(resp.GetServices(), networkName)
	if len(using) > 0 {
		return fmt.Errorf("network '%s' is in use by services: %v\nRemove services from the network first", networkName, using)
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"network_changes": map[string]any{
			networkName: nil,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted network '%s'\n", networkName)
	return nil
}
