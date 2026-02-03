package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var composeSetLabelCmd = &cobra.Command{
	Use:   "set-label <server-id> <stack-name> <service> <key=value>",
	Short: "Set a label for a service",
	Long: `Set or update a label for a service in a Docker Compose stack.

Examples:
  # Set a label
  berth-cli compose set-label 1 my-stack nginx app.version=1.2.3

  # Set deployment metadata
  berth-cli compose set-label 1 my-stack nginx deploy.commit=abc123 --yes`,
	Args: cobra.ExactArgs(4),
	RunE: runComposeSetLabel,
}

var composeUnsetLabelCmd = &cobra.Command{
	Use:   "unset-label <server-id> <stack-name> <service> <key>",
	Short: "Remove a label from a service",
	Long: `Remove a label from a service in a Docker Compose stack.

Examples:
  # Remove a label
  berth-cli compose unset-label 1 my-stack nginx deprecated

  # Remove with confirmation skip
  berth-cli compose unset-label 1 my-stack nginx old-label --yes`,
	Args: cobra.ExactArgs(4),
	RunE: runComposeUnsetLabel,
}

func init() {
	composeCmd.AddCommand(composeSetLabelCmd)
	composeCmd.AddCommand(composeUnsetLabelCmd)
	composeSetLabelCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeUnsetLabelCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func runComposeSetLabel(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("invalid format: expected key=value, got '%s'", keyValue)
	}
	key, value := parts[0], parts[1]

	if key == "" {
		return fmt.Errorf("label key cannot be empty")
	}

	serviceChanges := berth.NewServiceChanges()
	serviceChanges.SetLabels(map[string]string{
		key: value,
	})

	changes := berth.NewComposeChanges()
	changes.SetServiceChanges(map[string]berth.ServiceChanges{
		serviceName: *serviceChanges,
	})

	if err := updateComposeWithConfirm(c, serverID, stackName, changes, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully set label '%s' for service '%s'\n", key, serviceName)
	return nil
}

func runComposeUnsetLabel(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("label key cannot be empty")
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"labels": map[string]any{
					key: nil,
				},
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully removed label '%s' from service '%s'\n", key, serviceName)
	return nil
}
