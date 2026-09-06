package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var composeCreateSecretCmd = &cobra.Command{
	Use:   "create-secret <secret-name>",
	Short: "Create a secret in a stack",
	Long: `Create a new secret definition in a Docker Compose stack.

Secrets can be sourced from a file or environment variable.

Examples:
  # Create secret from file
  berth-cli compose create-secret -s 1 -n my-stack db_password --file ./secrets/db_password.txt

  # Create secret from environment variable
  berth-cli compose create-secret -s 1 -n my-stack db_password --environment DB_PASSWORD

  # Create an external secret reference
  berth-cli compose create-secret -s 1 -n my-stack existing-secret --external

  # Skip confirmation
  berth-cli compose create-secret -s 1 -n my-stack api_key --file ./secret.txt --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runComposeCreateSecret,
}

var composeDeleteSecretCmd = &cobra.Command{
	Use:   "delete-secret <secret-name>",
	Short: "Delete a secret from a stack",
	Long: `Delete a secret definition from a Docker Compose stack.

Examples:
  # Delete a secret
  berth-cli compose delete-secret -s 1 -n my-stack old-secret

  # Skip confirmation
  berth-cli compose delete-secret -s 1 -n my-stack unused-secret --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runComposeDeleteSecret,
}

func init() {
	composeCmd.AddCommand(composeCreateSecretCmd)
	composeCmd.AddCommand(composeDeleteSecretCmd)

	composeCreateSecretCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
	composeCreateSecretCmd.Flags().String("file", "", "Path to secret file")
	composeCreateSecretCmd.Flags().String("environment", "", "Environment variable containing the secret")
	composeCreateSecretCmd.Flags().Bool("external", false, "Mark as external secret")

	composeDeleteSecretCmd.Flags().BoolP("yes", "y", false, "Skip confirmation and apply immediately")
}

func stackSecretExists(secrets map[string]map[string]any, secretName string) bool {
	_, exists := secrets[secretName]
	return exists
}

func runComposeCreateSecret(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	secretName := args[0]
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	file, _ := cmd.Flags().GetString("file")
	environment, _ := cmd.Flags().GetString("environment")
	external, _ := cmd.Flags().GetBool("external")

	if file == "" && environment == "" && !external {
		return fmt.Errorf("must specify --file, --environment, or --external")
	}

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	data := resp.GetData()

	if stackSecretExists(data.GetSecrets(), secretName) {
		return fmt.Errorf("secret '%s' already exists in stack", secretName)
	}

	secretConfig := map[string]any{}
	if file != "" {
		secretConfig["file"] = file
	}
	if environment != "" {
		secretConfig["environment"] = environment
	}
	if external {
		secretConfig["external"] = true
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"secret_changes": map[string]any{
			secretName: secretConfig,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully created secret '%s'\n", secretName)
	return nil
}

func runComposeDeleteSecret(cmd *cobra.Command, args []string) error {
	c, err := newClient(cmd)
	if err != nil {
		return err
	}

	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}

	stackName := getStackName(cmd)
	secretName := args[0]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	resp, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposeGet(c.Ctx, serverID, stackName).Execute()
	if err != nil {
		return fmt.Errorf("failed to get compose config: %w", err)
	}

	data := resp.GetData()

	if !stackSecretExists(data.GetSecrets(), secretName) {
		return fmt.Errorf("secret '%s' not found in stack", secretName)
	}

	if err := updateComposeRaw(c, serverID, stackName, map[string]any{
		"secret_changes": map[string]any{
			secretName: nil,
		},
	}, skipConfirm); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted secret '%s'\n", secretName)
	return nil
}
