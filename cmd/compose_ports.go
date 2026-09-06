package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var composeAddPortCmd = &cobra.Command{
	Use:   "add-port <service> <port-mapping>",
	Short: "Add a port mapping to a service",
	Long: `Add a port mapping to a service in a Docker Compose stack.

Port format: [host_ip:]published:target[/protocol]

Examples:
  # Add basic port mapping (host:container)
  berth-cli compose add-port -s 1 -n my-stack nginx 8080:80

  # Add port with protocol
  berth-cli compose add-port -s 1 -n my-stack nginx 443:443/tcp

  # Add port with host IP binding
  berth-cli compose add-port -s 1 -n my-stack nginx 127.0.0.1:8080:80

  # Skip confirmation
  berth-cli compose add-port -s 1 -n my-stack nginx 8080:80 --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runComposeAddPort,
}

var composeRemovePortCmd = &cobra.Command{
	Use:   "remove-port <service> <port-mapping>",
	Short: "Remove a port mapping from a service",
	Long: `Remove a port mapping from a service in a Docker Compose stack.

Port format: [host_ip:]published:target[/protocol]

Examples:
  # Remove port mapping
  berth-cli compose remove-port -s 1 -n my-stack nginx 8080:80

  # Skip confirmation
  berth-cli compose remove-port -s 1 -n my-stack nginx 8080:80 --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runComposeRemovePort,
}

func init() {
	composeCmd.AddCommand(composeAddPortCmd)
	composeCmd.AddCommand(composeRemovePortCmd)
	composeAddPortCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeRemovePortCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

type portMapping struct {
	HostIP    string
	Published string
	Target    string
	Protocol  string
}

func parsePortMapping(s string) (*portMapping, error) {
	pm := &portMapping{Protocol: "tcp"}

	if idx := strings.LastIndex(s, "/"); idx != -1 {
		pm.Protocol = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		pm.Published = parts[0]
		pm.Target = parts[1]
	case 3:
		pm.HostIP = parts[0]
		pm.Published = parts[1]
		pm.Target = parts[2]
	default:
		return nil, fmt.Errorf("invalid port format: %s (expected [host_ip:]published:target[/protocol])", s)
	}

	return pm, nil
}

func portToMap(pm *portMapping) map[string]any {
	m := map[string]any{
		"target":    pm.Target,
		"published": pm.Published,
	}
	if pm.HostIP != "" {
		m["host_ip"] = pm.HostIP
	}
	if pm.Protocol != "" && pm.Protocol != "tcp" {
		m["protocol"] = pm.Protocol
	}
	return m
}

func getTargetPort(m map[string]any) string {
	switch v := m["target"].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

func portsMatch(a, b map[string]any) bool {
	aTarget := getTargetPort(a)
	bTarget := getTargetPort(b)

	aPublished, _ := a["published"].(string)
	bPublished, _ := b["published"].(string)

	return aTarget == bTarget && aPublished == bPublished
}

func normalizePortEntry(p any) (map[string]any, error) {
	switch v := p.(type) {
	case map[string]any:
		return v, nil
	case string:
		pm, err := parsePortMapping(v)
		if err != nil {
			return nil, err
		}
		return portToMap(pm), nil
	case float64:
		return map[string]any{
			"target":    strconv.FormatFloat(v, 'f', -1, 64),
			"published": "",
		}, nil
	case int:
		return map[string]any{
			"target":    strconv.Itoa(v),
			"published": "",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported port format: %T", p)
	}
}

func getCurrentPorts(services map[string]map[string]any, serviceName string) ([]map[string]any, error) {
	service, ok := services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service '%s' not found", serviceName)
	}

	portsRaw, ok := service["ports"]
	if !ok || portsRaw == nil {
		return []map[string]any{}, nil
	}

	portsSlice, ok := portsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid ports format")
	}

	var ports []map[string]any
	for _, p := range portsSlice {
		normalized, err := normalizePortEntry(p)
		if err != nil {
			return nil, fmt.Errorf("invalid port entry: %v", err)
		}
		ports = append(ports, normalized)
	}

	return ports, nil
}

func runComposeAddPort(cmd *cobra.Command, args []string) error {
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
	portStr := args[1]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	newPort, err := parsePortMapping(portStr)
	if err != nil {
		return err
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	data := resp.GetData()

	currentPorts, err := getCurrentPorts(data.GetServices(), serviceName)
	if err != nil {
		return err
	}

	newPortMap := portToMap(newPort)
	for _, p := range currentPorts {
		if portsMatch(p, newPortMap) {
			return fmt.Errorf("port mapping %s already exists", portStr)
		}
	}

	currentPorts = append(currentPorts, newPortMap)

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"ports": currentPorts,
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully added port %s to service '%s'\n", portStr, serviceName)
	return nil
}

func runComposeRemovePort(cmd *cobra.Command, args []string) error {
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
	portStr := args[1]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	targetPort, err := parsePortMapping(portStr)
	if err != nil {
		return err
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	data := resp.GetData()

	currentPorts, err := getCurrentPorts(data.GetServices(), serviceName)
	if err != nil {
		return err
	}

	targetPortMap := portToMap(targetPort)
	newPorts := make([]map[string]any, 0)
	found := false
	for _, p := range currentPorts {
		if portsMatch(p, targetPortMap) {
			found = true
			continue
		}
		newPorts = append(newPorts, p)
	}

	if !found {
		return fmt.Errorf("port mapping %s not found", portStr)
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"ports": newPorts,
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully removed port %s from service '%s'\n", portStr, serviceName)
	return nil
}
