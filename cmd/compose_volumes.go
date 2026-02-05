package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var composeAddVolumeCmd = &cobra.Command{
	Use:   "add-volume <service> <volume-mount>",
	Short: "Add a volume mount to a service",
	Long: `Add a volume mount to a service in a Docker Compose stack.

Volume format: source:target[:ro]
  - Bind mount: ./path:/container/path or /host/path:/container/path
  - Named volume: volume-name:/container/path
  - Read-only: append :ro

Examples:
  # Add bind mount
  berth-cli compose add-volume -s 1 -n my-stack nginx ./html:/usr/share/nginx/html

  # Add named volume
  berth-cli compose add-volume -s 1 -n my-stack db data:/var/lib/mysql

  # Add read-only mount
  berth-cli compose add-volume -s 1 -n my-stack app ./config:/app/config:ro

  # Skip confirmation
  berth-cli compose add-volume -s 1 -n my-stack nginx ./logs:/var/log/nginx --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runComposeAddVolume,
}

var composeRemoveVolumeCmd = &cobra.Command{
	Use:   "remove-volume <service> <target-path>",
	Short: "Remove a volume mount from a service",
	Long: `Remove a volume mount from a service by its container target path.

Examples:
  # Remove volume by target path
  berth-cli compose remove-volume -s 1 -n my-stack nginx /usr/share/nginx/html

  # Skip confirmation
  berth-cli compose remove-volume -s 1 -n my-stack nginx /var/log/nginx --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runComposeRemoveVolume,
}

func init() {
	composeCmd.AddCommand(composeAddVolumeCmd)
	composeCmd.AddCommand(composeRemoveVolumeCmd)
	composeAddVolumeCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeRemoveVolumeCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

type volumeMount struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

func parseVolumeMount(s string) (*volumeMount, error) {
	vm := &volumeMount{}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		vm.Source = parts[0]
		vm.Target = parts[1]
	case 3:
		vm.Source = parts[0]
		vm.Target = parts[1]
		if parts[2] == "ro" {
			vm.ReadOnly = true
		} else {
			return nil, fmt.Errorf("invalid volume option: %s (expected 'ro')", parts[2])
		}
	default:
		return nil, fmt.Errorf("invalid volume format: %s (expected source:target[:ro])", s)
	}

	if strings.HasPrefix(vm.Source, "./") || strings.HasPrefix(vm.Source, "/") || strings.HasPrefix(vm.Source, "../") {
		vm.Type = "bind"
	} else {
		vm.Type = "volume"
	}

	return vm, nil
}

func volumeToMap(vm *volumeMount) map[string]any {
	m := map[string]any{
		"type":   vm.Type,
		"source": vm.Source,
		"target": vm.Target,
	}
	if vm.ReadOnly {
		m["read_only"] = true
	}
	return m
}

func normalizeVolumeEntry(v any) (map[string]any, error) {
	switch val := v.(type) {
	case map[string]any:
		return val, nil
	case string:
		vm, err := parseVolumeMount(val)
		if err != nil {
			return nil, err
		}
		return volumeToMap(vm), nil
	default:
		return nil, fmt.Errorf("unsupported volume format: %T", v)
	}
}

func getCurrentVolumes(services map[string]map[string]any, serviceName string) ([]map[string]any, error) {
	service, ok := services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service '%s' not found", serviceName)
	}

	volumesRaw, ok := service["volumes"]
	if !ok || volumesRaw == nil {
		return []map[string]any{}, nil
	}

	volumesSlice, ok := volumesRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid volumes format")
	}

	var volumes []map[string]any
	for _, v := range volumesSlice {
		normalized, err := normalizeVolumeEntry(v)
		if err != nil {
			return nil, fmt.Errorf("invalid volume entry: %v", err)
		}
		volumes = append(volumes, normalized)
	}

	return volumes, nil
}

func getVolumeTarget(m map[string]any) string {
	target, _ := m["target"].(string)
	return target
}

func runComposeAddVolume(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	serviceName := args[0]
	volumeStr := args[1]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	newVolume, err := parseVolumeMount(volumeStr)
	if err != nil {
		return err
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	currentVolumes, err := getCurrentVolumes(resp.GetServices(), serviceName)
	if err != nil {
		return err
	}

	for _, v := range currentVolumes {
		if getVolumeTarget(v) == newVolume.Target {
			return fmt.Errorf("volume mount to '%s' already exists", newVolume.Target)
		}
	}

	currentVolumes = append(currentVolumes, volumeToMap(newVolume))

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"volumes": currentVolumes,
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully added volume mount to '%s' for service '%s'\n", newVolume.Target, serviceName)
	return nil
}

func runComposeRemoveVolume(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	serviceName := args[0]
	targetPath := args[1]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	currentVolumes, err := getCurrentVolumes(resp.GetServices(), serviceName)
	if err != nil {
		return err
	}

	newVolumes := make([]map[string]any, 0)
	found := false
	for _, v := range currentVolumes {
		if getVolumeTarget(v) == targetPath {
			found = true
			continue
		}
		newVolumes = append(newVolumes, v)
	}

	if !found {
		return fmt.Errorf("volume mount to '%s' not found", targetPath)
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"volumes": newVolumes,
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully removed volume mount to '%s' from service '%s'\n", targetPath, serviceName)
	return nil
}
