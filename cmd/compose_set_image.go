package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var composeSetImageCmd = &cobra.Command{
	Use:   "set-image <service> <image>",
	Short: "Set the image for a service",
	Long: `Update the container image for a service in a Docker Compose stack.

Examples:
  # Set image with preview and confirmation
  berth-cli compose set-image -s 1 -n my-stack nginx nginx:1.25

  # Skip confirmation and apply directly
  berth-cli compose set-image -s 1 -n my-stack nginx nginx:1.25 --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runComposeSetImage,
}

func init() {
	composeCmd.AddCommand(composeSetImageCmd)
	composeSetImageCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func runComposeSetImage(cmd *cobra.Command, args []string) error {
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

	serviceChanges := berth.NewServiceChanges()
	serviceChanges.SetImage(image)

	changes := berth.NewComposeChanges()
	changes.SetServiceChanges(map[string]berth.ServiceChanges{
		serviceName: *serviceChanges,
	})

	if err := updateComposeWithConfirm(c, serverID, stackName, changes, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully updated image for service '%s' to '%s'\n", serviceName, image)
	return nil
}
