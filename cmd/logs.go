package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/Tech-Arch1tect/berth-cli/pkg/output"
	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var logsCmd = &cobra.Command{
	Use:         "logs",
	Short:       "Read a finite stack or container log snapshot",
	Long:        "Read a finite log snapshot for a stack or an exact API container name, without following or discovering containers.",
	Args:        cobra.NoArgs,
	RunE:        runLogs,
	Annotations: mcpMetadata("read"),
}

func init() {
	rootCmd.AddCommand(logsCmd)
	addServerIDFlag(logsCmd, false)
	addStackFlag(logsCmd, false)
	logsCmd.Flags().String("container", "", "Select an exact API container name")
	logsCmd.Flags().Int32("tail", 100, "Number of log lines to request (positive int32)")
	logsCmd.Flags().String("since", "", "Request logs since an RFC3339 timestamp or positive Go duration")
	logsCmd.Flags().Bool("timestamps", true, "Request timestamps and include them in text output")
	logsCmd.Flags().StringP("output", "o", "text", "Output format: text, json")
}

func runLogs(cmd *cobra.Command, args []string) error {
	serverID, err := getServerID(cmd)
	if err != nil {
		return err
	}
	stackName := getStackName(cmd)
	if stackName == "" {
		return fmt.Errorf("stack name must not be empty")
	}
	containerName, _ := cmd.Flags().GetString("container")
	if cmd.Flags().Changed("container") && containerName == "" {
		return fmt.Errorf("container name must not be empty")
	}
	tail, _ := cmd.Flags().GetInt32("tail")
	if tail <= 0 {
		return fmt.Errorf("tail must be a positive whole number within int32 range")
	}
	since, _ := cmd.Flags().GetString("since")
	if cmd.Flags().Changed("since") {
		if _, err := time.Parse(time.RFC3339Nano, since); err != nil {
			duration, err := time.ParseDuration(since)
			if err != nil || duration <= 0 {
				return fmt.Errorf("since must be an RFC3339 timestamp or a positive Go duration")
			}
			since = time.Now().Add(-duration).UTC().Format(time.RFC3339Nano)
		}
	}
	timestamps, _ := cmd.Flags().GetBool("timestamps")
	format, _ := cmd.Flags().GetString("output")
	if format != "text" && format != "json" {
		return fmt.Errorf("unknown output format %q: use text or json", format)
	}
	c, err := newClient(cmd)
	if err != nil {
		return err
	}
	var resp *berth.ResponseLogsData
	if containerName == "" {
		request := c.API.LogsAPI.ApiV1ServersServeridStacksStacknameLogsGet(c.Ctx, serverID, stackName).Tail(tail).Timestamps(timestamps)
		if since != "" {
			request = request.Since(since)
		}
		resp, _, err = request.Execute()
	} else {
		request := c.API.LogsAPI.ApiV1ServersServeridStacksStacknameContainersContainerNameLogsGet(c.Ctx, serverID, stackName, containerName).Tail(tail).Timestamps(timestamps)
		if since != "" {
			request = request.Since(since)
		}
		resp, _, err = request.Execute()
	}
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("failed to get logs: API returned an empty response")
	}
	if !resp.Success {
		return fmt.Errorf("failed to get logs: API returned success=false")
	}
	data := resp.GetData()
	if format == "json" {
		return output.New(output.FormatJSON).Print(os.Stdout, data)
	}
	for _, entry := range data.Logs {
		if timestamps {
			_, err = fmt.Fprintf(os.Stdout, "%s %s %s\n", entry.Timestamp.Format(time.RFC3339Nano), entry.Source, entry.Message)
		} else {
			_, err = fmt.Fprintf(os.Stdout, "%s %s\n", entry.Source, entry.Message)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
