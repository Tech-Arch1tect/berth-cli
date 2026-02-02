package cmd

import (
	"github.com/Tech-Arch1tect/berth-cli/pkg/client"
	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	"github.com/Tech-Arch1tect/berth-cli/pkg/output"
	"github.com/spf13/cobra"
)

var (
	flagAPIKey   string
	flagServer   string
	flagInsecure bool
	flagOutput   string
	flagVerbose  bool
)

var rootCmd = &cobra.Command{
	Use:   "berth-cli",
	Short: "Berth CLI - Manage Docker stacks remotely",
	Long: `A command-line interface for managing Docker stacks through the Berth platform.

Configuration is loaded from (in priority order):
  1. CLI flags
  2. Environment variables (BERTH_SERVER_URL, BERTH_API_KEY, BERTH_INSECURE)
  3. ./berth-cli.yaml
  4. ~/.config/berth-cli/config.yaml
  5. ~/.berth-cli.yaml`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "Berth API key")
	rootCmd.PersistentFlags().StringVar(&flagServer, "server", "", "Berth server URL")
	rootCmd.PersistentFlags().BoolVar(&flagInsecure, "insecure", false, "Skip TLS certificate verification")
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table, json")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose output (show HTTP requests)")
}

func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	insecureSet := cmd.Flags().Changed("insecure")
	return config.Load(flagServer, flagAPIKey, flagInsecure, insecureSet, flagVerbose)
}

func newClient(cmd *cobra.Command) (*client.Client, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return nil, err
	}
	return client.New(cfg), nil
}

func getOutputFormat() (output.Format, error) {
	return output.ParseFormat(flagOutput)
}
