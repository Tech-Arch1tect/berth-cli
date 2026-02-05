package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var composeAddServiceCmd = &cobra.Command{
	Use:   "add-service <service-name> <image>",
	Short: "Add a new service to a stack",
	Long: `Add a new service to a Docker Compose stack.

The service is created with minimal configuration. Use other compose commands
to add ports, volumes, environment variables, etc. after creation.

Examples:
  # Add a basic service
  berth-cli compose add-service -s 1 -n my-stack redis redis:7-alpine

  # Add service with restart policy
  berth-cli compose add-service -s 1 -n my-stack nginx nginx:alpine --restart always

  # Skip confirmation
  berth-cli compose add-service -s 1 -n my-stack api myapp:latest --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runComposeAddService,
}

var composeRemoveServiceCmd = &cobra.Command{
	Use:   "remove-service <service-name>",
	Short: "Remove a service from a stack",
	Long: `Remove a service from a Docker Compose stack.

This permanently removes the service definition from the compose file.

Examples:
  # Remove a service
  berth-cli compose remove-service -s 1 -n my-stack old-service

  # Skip confirmation
  berth-cli compose remove-service -s 1 -n my-stack old-service --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runComposeRemoveService,
}

var composeRenameServiceCmd = &cobra.Command{
	Use:   "rename-service <old-name> <new-name>",
	Short: "Rename a service in a stack",
	Long: `Rename a service in a Docker Compose stack.

This updates the service name while preserving all configuration.

Examples:
  # Rename a service
  berth-cli compose rename-service -s 1 -n my-stack old-name new-name

  # Skip confirmation
  berth-cli compose rename-service -s 1 -n my-stack web frontend --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runComposeRenameService,
}

func init() {
	composeCmd.AddCommand(composeAddServiceCmd)
	composeCmd.AddCommand(composeRemoveServiceCmd)
	composeCmd.AddCommand(composeRenameServiceCmd)

	composeAddServiceCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeAddServiceCmd.Flags().String("restart", "", "Restart policy (no, always, on-failure, unless-stopped)")

	composeRemoveServiceCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeRenameServiceCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func serviceExists(services map[string]map[string]any, serviceName string) bool {
	_, exists := services[serviceName]
	return exists
}

func runComposeAddService(cmd *cobra.Command, args []string) error {
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
	image := args[1]
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	restart, _ := cmd.Flags().GetString("restart")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	if serviceExists(resp.GetServices(), serviceName) {
		return fmt.Errorf("service '%s' already exists in stack", serviceName)
	}

	newService := map[string]any{
		"image": image,
	}
	if restart != "" {
		newService["restart"] = restart
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"add_services": map[string]any{
			serviceName: newService,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully added service '%s' with image '%s'\n", serviceName, image)
	return nil
}

func runComposeRemoveService(cmd *cobra.Command, args []string) error {
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
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	services := resp.GetServices()
	if !serviceExists(services, serviceName) {
		return fmt.Errorf("service '%s' not found in stack", serviceName)
	}

	if len(services) == 1 {
		return fmt.Errorf("cannot remove the last service from a stack")
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"delete_services": []string{serviceName},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully removed service '%s'\n", serviceName)
	return nil
}

func runComposeRenameService(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	oldName := args[0]
	newName := args[1]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if oldName == newName {
		return fmt.Errorf("old name and new name are the same")
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	services := resp.GetServices()
	if !serviceExists(services, oldName) {
		return fmt.Errorf("service '%s' not found in stack", oldName)
	}

	if serviceExists(services, newName) {
		return fmt.Errorf("service '%s' already exists in stack", newName)
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"rename_services": map[string]string{
			oldName: newName,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully renamed service '%s' to '%s'\n", oldName, newName)
	return nil
}
