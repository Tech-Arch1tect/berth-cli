package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var composeCreateVolumeCmd = &cobra.Command{
	Use:   "create-volume <server-id> <stack-name> <volume-name>",
	Short: "Create a volume in a stack",
	Long: `Create a new volume definition in a Docker Compose stack.

The volume is created with default settings. Use flags to customize.

Examples:
  # Create a basic volume
  berth-cli compose create-volume 1 my-stack data

  # Create with custom driver
  berth-cli compose create-volume 1 my-stack logs --driver local

  # Create with driver options
  berth-cli compose create-volume 1 my-stack shared --driver-opt type=nfs --driver-opt o=addr=10.0.0.1

  # Create an external volume reference
  berth-cli compose create-volume 1 my-stack existing-vol --external

  # Skip confirmation
  berth-cli compose create-volume 1 my-stack cache --yes`,
	Args: cobra.ExactArgs(3),
	RunE: runComposeCreateVolume,
}

var composeDeleteVolumeCmd = &cobra.Command{
	Use:   "delete-volume <server-id> <stack-name> <volume-name>",
	Short: "Delete a volume from a stack",
	Long: `Delete a volume definition from a Docker Compose stack.

The volume must not be in use by any services.

Examples:
  # Delete a volume
  berth-cli compose delete-volume 1 my-stack old-volume

  # Skip confirmation
  berth-cli compose delete-volume 1 my-stack unused-vol --yes`,
	Args: cobra.ExactArgs(3),
	RunE: runComposeDeleteVolume,
}

func init() {
	composeCmd.AddCommand(composeCreateVolumeCmd)
	composeCmd.AddCommand(composeDeleteVolumeCmd)

	composeCreateVolumeCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeCreateVolumeCmd.Flags().String("driver", "", "Volume driver (e.g., local, nfs)")
	composeCreateVolumeCmd.Flags().StringArray("driver-opt", nil, "Driver options (can be specified multiple times: --driver-opt key=value)")
	composeCreateVolumeCmd.Flags().Bool("external", false, "Mark as external volume")

	composeDeleteVolumeCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func stackVolumeExists(volumes map[string]map[string]any, volumeName string) bool {
	_, exists := volumes[volumeName]
	return exists
}

func getServicesUsingStackVolume(services map[string]map[string]any, volumeName string) []string {
	var using []string
	for serviceName, svc := range services {
		if volumes, ok := svc["volumes"].([]any); ok {
			for _, v := range volumes {
				if vol, ok := v.(map[string]any); ok {
					if volType, _ := vol["type"].(string); volType == "volume" {
						if source, _ := vol["source"].(string); source == volumeName {
							using = append(using, serviceName)
							break
						}
					}
				}
			}
		}
	}
	return using
}

func parseDriverOpts(opts []string) (map[string]string, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	result := make(map[string]string)
	for _, opt := range opts {
		parts := strings.SplitN(opt, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid driver option format: %s (expected key=value)", opt)
		}
		result[parts[0]] = parts[1]
	}
	return result, nil
}

func runComposeCreateVolume(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	var serverID int32
	if _, err := fmt.Sscanf(args[0], "%d", &serverID); err != nil {
		return fmt.Errorf("invalid server ID: %s", args[0])
	}

	stackName := args[1]
	volumeName := args[2]
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	driver, _ := cmd.Flags().GetString("driver")
	driverOpts, _ := cmd.Flags().GetStringArray("driver-opt")
	external, _ := cmd.Flags().GetBool("external")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	if stackVolumeExists(resp.GetVolumes(), volumeName) {
		return fmt.Errorf("volume '%s' already exists in stack", volumeName)
	}

	volConfig := map[string]any{}
	if driver != "" {
		volConfig["driver"] = driver
	}
	if external {
		volConfig["external"] = true
	}

	if len(driverOpts) > 0 {
		opts, err := parseDriverOpts(driverOpts)
		if err != nil {
			return err
		}
		volConfig["driver_opts"] = opts
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"volume_changes": map[string]any{
			volumeName: volConfig,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully created volume '%s'\n", volumeName)
	return nil
}

func runComposeDeleteVolume(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	var serverID int32
	if _, err := fmt.Sscanf(args[0], "%d", &serverID); err != nil {
		return fmt.Errorf("invalid server ID: %s", args[0])
	}

	stackName := args[1]
	volumeName := args[2]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	if !stackVolumeExists(resp.GetVolumes(), volumeName) {
		return fmt.Errorf("volume '%s' not found in stack", volumeName)
	}

	using := getServicesUsingStackVolume(resp.GetServices(), volumeName)
	if len(using) > 0 {
		return fmt.Errorf("volume '%s' is in use by services: %v\nRemove volume mounts from services first", volumeName, using)
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"volume_changes": map[string]any{
			volumeName: nil,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted volume '%s'\n", volumeName)
	return nil
}
