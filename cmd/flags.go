package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func addServerIDFlag(cmd *cobra.Command, persistent bool) {
	if persistent {
		cmd.PersistentFlags().StringP("server-id", "s", "", "Server ID (required)")
		cmd.MarkPersistentFlagRequired("server-id")
	} else {
		cmd.Flags().StringP("server-id", "s", "", "Server ID (required)")
		cmd.MarkFlagRequired("server-id")
	}
}

func addStackFlag(cmd *cobra.Command, persistent bool) {
	if persistent {
		cmd.PersistentFlags().StringP("stack", "n", "", "Stack name (required)")
		cmd.MarkPersistentFlagRequired("stack")
	} else {
		cmd.Flags().StringP("stack", "n", "", "Stack name (required)")
		cmd.MarkFlagRequired("stack")
	}
}

func getServerID(cmd *cobra.Command) (int32, error) {
	s, err := cmd.Flags().GetString("server-id")
	if err != nil {
		return 0, fmt.Errorf("server-id flag not found: %w", err)
	}
	var id int32
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid server ID: %s", s)
	}
	return id, nil
}

func getServerIDStr(cmd *cobra.Command) string {
	s, _ := cmd.Flags().GetString("server-id")
	return s
}

func getStackName(cmd *cobra.Command) string {
	s, _ := cmd.Flags().GetString("stack")
	return s
}
