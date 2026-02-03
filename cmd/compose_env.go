package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var composeSetEnvCmd = &cobra.Command{
	Use:   "set-env <server-id> <stack-name> <service> <KEY=value>",
	Short: "Set an environment variable for a service",
	Long: `Set or update an environment variable for a service in a Docker Compose stack.

Examples:
  # Set an environment variable
  berth-cli compose set-env 1 my-stack nginx DATABASE_URL=postgres://localhost/db

  # Set with confirmation skip
  berth-cli compose set-env 1 my-stack nginx API_KEY=secret --yes`,
	Args: cobra.ExactArgs(4),
	RunE: runComposeSetEnv,
}

var composeUnsetEnvCmd = &cobra.Command{
	Use:   "unset-env <server-id> <stack-name> <service> <KEY>",
	Short: "Remove an environment variable from a service",
	Long: `Remove an environment variable from a service in a Docker Compose stack.

Examples:
  # Remove an environment variable
  berth-cli compose unset-env 1 my-stack nginx DEBUG

  # Remove with confirmation skip
  berth-cli compose unset-env 1 my-stack nginx DEBUG --yes`,
	Args: cobra.ExactArgs(4),
	RunE: runComposeUnsetEnv,
}

func init() {
	composeCmd.AddCommand(composeSetEnvCmd)
	composeCmd.AddCommand(composeUnsetEnvCmd)
	composeSetEnvCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeUnsetEnvCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func runComposeSetEnv(cmd *cobra.Command, args []string) error {
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
	keyValue := args[3]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	parts := strings.SplitN(keyValue, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format: expected KEY=value, got '%s'", keyValue)
	}
	key, value := parts[0], parts[1]

	if key == "" {
		return fmt.Errorf("environment variable key cannot be empty")
	}

	serviceChanges := berth.NewServiceChanges()
	serviceChanges.SetEnvironment(map[string]string{
		key: value,
	})

	changes := berth.NewComposeChanges()
	changes.SetServiceChanges(map[string]berth.ServiceChanges{
		serviceName: *serviceChanges,
	})

	if err := updateComposeWithConfirm(c, serverID, stackName, changes, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully set environment variable '%s' for service '%s'\n", key, serviceName)
	return nil
}

func runComposeUnsetEnv(cmd *cobra.Command, args []string) error {
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
	key := args[3]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if key == "" {
		return fmt.Errorf("environment variable key cannot be empty")
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"environment": map[string]any{
					key: nil,
				},
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully removed environment variable '%s' from service '%s'\n", key, serviceName)
	return nil
}
