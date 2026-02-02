package cmd

import (
	"github.com/spf13/cobra"
)

var composeCmd = &cobra.Command{
	Use:   "compose",
	Short: "Manage Docker Compose configurations",
	Long:  `View and modify Docker Compose stack configurations.`,
}

func init() {
	rootCmd.AddCommand(composeCmd)
}
