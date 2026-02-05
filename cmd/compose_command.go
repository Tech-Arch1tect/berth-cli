package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var composeSetCommandCmd = &cobra.Command{
	Use:   "set-command <service> -- <command...>",
	Short: "Set the command for a service",
	Long: `Set the command for a service in a Docker Compose stack.

Use -- to separate the command arguments. All flags must come BEFORE --.

Examples:
  # Set command
  berth-cli compose set-command -s 1 -n my-stack app -- npm run start

  # Set command with multiple arguments
  berth-cli compose set-command -s 1 -n my-stack app -- /bin/sh -c "echo hello"

  # Skip confirmation (flags before --)
  berth-cli compose set-command --yes -s 1 -n my-stack app -- python app.py`,
	Args: cobra.MinimumNArgs(1),
	RunE: runComposeSetCommand,
}

var composeSetEntrypointCmd = &cobra.Command{
	Use:   "set-entrypoint <service> -- <entrypoint...>",
	Short: "Set the entrypoint for a service",
	Long: `Set the entrypoint for a service in a Docker Compose stack.

Use -- to separate the entrypoint arguments. All flags must come BEFORE --.

Examples:
  # Set entrypoint
  berth-cli compose set-entrypoint -s 1 -n my-stack app -- /docker-entrypoint.sh

  # Set entrypoint with arguments
  berth-cli compose set-entrypoint -s 1 -n my-stack app -- /bin/sh -c

  # Skip confirmation (flags before --)
  berth-cli compose set-entrypoint --yes -s 1 -n my-stack app -- /entrypoint.sh`,
	Args: cobra.MinimumNArgs(1),
	RunE: runComposeSetEntrypoint,
}

func init() {
	composeCmd.AddCommand(composeSetCommandCmd)
	composeCmd.AddCommand(composeSetEntrypointCmd)
	composeSetCommandCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeSetEntrypointCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func runComposeSetCommand(cmd *cobra.Command, args []string) error {
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

	var command []string
	if len(args) > 1 {
		command = args[1:]
	}

	if len(command) == 0 {
		return fmt.Errorf("command cannot be empty. Use -- to separate command arguments")
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"command": map[string]any{
					"values": command,
				},
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully set command for service '%s'\n", serviceName)
	return nil
}

func runComposeSetEntrypoint(cmd *cobra.Command, args []string) error {
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

	var entrypoint []string
	if len(args) > 1 {
		entrypoint = args[1:]
	}

	if len(entrypoint) == 0 {
		return fmt.Errorf("entrypoint cannot be empty. Use -- to separate entrypoint arguments")
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"service_changes": map[string]any{
			serviceName: map[string]any{
				"entrypoint": map[string]any{
					"values": entrypoint,
				},
			},
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully set entrypoint for service '%s'\n", serviceName)
	return nil
}
