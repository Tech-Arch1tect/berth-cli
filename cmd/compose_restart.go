package cmd

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var validRestartPolicies = []string{"no", "always", "on-failure", "unless-stopped"}

var composeSetRestartCmd = &cobra.Command{
	Use:   "set-restart <server-id> <stack-name> <service> <policy>",
	Short: "Set the restart policy for a service",
	Long: `Set the restart policy for a service in a Docker Compose stack.

Valid policies: no, always, on-failure, unless-stopped

Examples:
  # Set restart policy to always
  berth-cli compose set-restart 1 my-stack nginx always

  # Set restart policy to on-failure
  berth-cli compose set-restart 1 my-stack nginx on-failure --yes`,
	Args: cobra.ExactArgs(4),
	RunE: runComposeSetRestart,
}

func init() {
	composeCmd.AddCommand(composeSetRestartCmd)
	composeSetRestartCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func runComposeSetRestart(cmd *cobra.Command, args []string) error {
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
	policy := args[3]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if !slices.Contains(validRestartPolicies, policy) {
		return fmt.Errorf("invalid restart policy '%s'. Valid policies: %v", policy, validRestartPolicies)
	}

	serviceChanges := berth.NewServiceChanges()
	serviceChanges.SetRestart(policy)

	changes := berth.NewComposeChanges()
	changes.SetServiceChanges(map[string]berth.ServiceChanges{
		serviceName: *serviceChanges,
	})

	if err := updateComposeWithConfirm(c, serverID, stackName, changes, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully set restart policy to '%s' for service '%s'\n", policy, serviceName)
	return nil
}
