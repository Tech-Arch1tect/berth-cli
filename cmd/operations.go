package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/Tech-Arch1tect/berth-cli/pkg/output"
	"github.com/spf13/cobra"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

var operationsCmd = &cobra.Command{
	Use:   "operations",
	Short: "Read recorded Docker operation history",
}

var operationsListCmd = &cobra.Command{
	Use:         "list",
	Short:       "List one page of recorded Docker operations",
	Args:        cobra.NoArgs,
	RunE:        runOperationsList,
	Annotations: mcpMetadata("read"),
}

var operationsShowCmd = &cobra.Command{
	Use:         "show <operation-id>",
	Short:       "Read a recorded operation and its ordered messages",
	Args:        cobra.ExactArgs(1),
	RunE:        runOperationsShow,
	Annotations: mcpMetadata("read", "operation-id"),
}

func init() {
	rootCmd.AddCommand(operationsCmd)
	operationsCmd.AddCommand(operationsListCmd, operationsShowCmd)
	operationsListCmd.Flags().StringP("server-id", "s", "", "Filter by positive server ID")
	operationsListCmd.Flags().String("stack", "", "Filter by stack name substring")
	operationsListCmd.Flags().String("search", "", "Search recorded operation history")
	operationsListCmd.Flags().String("command", "", "Filter by exact recorded command")
	operationsListCmd.Flags().String("status", "", "Filter by complete, incomplete, failed or success")
	operationsListCmd.Flags().Int32("page", 1, "Page number (positive int32)")
	operationsListCmd.Flags().Int32("page-size", 20, "Records per page (1 to 100)")
}

type operationHistoryView struct {
	OperationID string     `json:"operation_id"`
	ServerID    int32      `json:"server_id"`
	ServerName  string     `json:"server_name"`
	StackName   string     `json:"stack_name"`
	Command     string     `json:"command"`
	Completion  string     `json:"completion"`
	Success     *bool      `json:"success"`
	ExitCode    *int32     `json:"exit_code"`
	StartTime   *time.Time `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
	DurationMs  *int32     `json:"duration_ms"`
	Options     *string    `json:"options,omitempty"`
	Services    *string    `json:"services,omitempty"`
	Summary     *string    `json:"summary,omitempty"`
}

type operationHistoryMessage struct {
	Timestamp      time.Time `json:"timestamp"`
	MessageType    string    `json:"message_type"`
	MessageData    string    `json:"message_data"`
	SequenceNumber int32     `json:"sequence_number"`
}

func operationHistory(log *berth.OperationLogInfo) operationHistoryView {
	view := operationHistoryView{
		OperationID: log.OperationId, ServerID: log.ServerId, ServerName: log.ServerName,
		StackName: log.StackName, Command: log.Command, Completion: "incomplete",
		Success: log.Success.Get(), ExitCode: log.ExitCode.Get(), StartTime: log.StartTime,
		EndTime: log.EndTime.Get(), Options: log.Options, Services: log.Services, Summary: log.Summary,
	}
	if view.EndTime != nil && !log.IsIncomplete {
		view.Completion = "complete"
		view.DurationMs = log.DurationMs.Get()
	}
	return view
}

func runOperationsList(cmd *cobra.Command, args []string) error {
	for _, name := range []string{"server-id", "stack", "search", "command", "status"} {
		value, _ := cmd.Flags().GetString(name)
		if cmd.Flags().Changed(name) && value == "" {
			return fmt.Errorf("%s must not be empty", name)
		}
	}
	if cmd.Flags().Changed("server-id") {
		if _, err := getServerID(cmd); err != nil {
			return err
		}
	}
	status, _ := cmd.Flags().GetString("status")
	if status != "" && status != "complete" && status != "incomplete" && status != "failed" && status != "success" {
		return fmt.Errorf("unknown status %q: use complete, incomplete, failed or success", status)
	}
	page, _ := cmd.Flags().GetInt32("page")
	if page <= 0 {
		return fmt.Errorf("page must be a positive whole number within int32 range")
	}
	pageSize, _ := cmd.Flags().GetInt32("page-size")
	if pageSize < 1 || pageSize > 100 {
		return fmt.Errorf("page-size must be between 1 and 100")
	}
	if flagOutput == "" {
		return fmt.Errorf("output format must not be empty: use table or json")
	}
	format, err := getOutputFormat()
	if err != nil {
		return err
	}
	c, err := newClient(cmd)
	if err != nil {
		return err
	}
	request := c.API.OperationLogsAPI.ApiV1OperationLogsGet(c.Ctx).Page(page).PageSize(pageSize)
	if value := getServerIDStr(cmd); value != "" {
		request = request.ServerId(value)
	}
	if value := getStackName(cmd); value != "" {
		request = request.StackName(value)
	}
	if value, _ := cmd.Flags().GetString("search"); value != "" {
		request = request.Search(value)
	}
	if value, _ := cmd.Flags().GetString("command"); value != "" {
		request = request.Command(value)
	}
	if status != "" {
		request = request.Status(status)
	}
	resp, _, err := request.Execute()
	if err != nil {
		return fmt.Errorf("failed to list operations: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("failed to list operations: API returned an empty response")
	}
	if !resp.Success {
		return fmt.Errorf("failed to list operations: API returned success=false")
	}
	views := make([]operationHistoryView, len(resp.Data))
	for i := range resp.Data {
		views[i] = operationHistory(&resp.Data[i])
	}
	if format == output.FormatJSON {
		return output.New(format).Print(os.Stdout, struct {
			Operations []operationHistoryView `json:"operations"`
			Meta       *berth.Meta            `json:"meta"`
		}{views, resp.Meta.Get()})
	}
	return printOperationHistory(views)
}

func runOperationsShow(cmd *cobra.Command, args []string) error {
	if args[0] == "" {
		return fmt.Errorf("operation ID must not be empty")
	}
	if flagOutput == "" {
		return fmt.Errorf("output format must not be empty: use table or json")
	}
	format, err := getOutputFormat()
	if err != nil {
		return err
	}
	c, err := newClient(cmd)
	if err != nil {
		return err
	}
	resp, _, err := c.API.OperationLogsAPI.ApiV1OperationLogsByOperationIdOperationIdGet(c.Ctx, args[0]).Execute()
	if err != nil {
		return fmt.Errorf("failed to show operation: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("failed to show operation: API returned an empty response")
	}
	if !resp.Success {
		return fmt.Errorf("failed to show operation: API returned success=false")
	}
	data := resp.GetData()
	view := operationHistory(&data.Log)
	if format == output.FormatJSON {
		messages := make([]operationHistoryMessage, len(data.Messages))
		for i := range data.Messages {
			message := &data.Messages[i]
			messages[i] = operationHistoryMessage{message.Timestamp, message.MessageType, message.MessageData, message.SequenceNumber}
		}
		return output.New(format).Print(os.Stdout, struct {
			Log      operationHistoryView      `json:"log"`
			Messages []operationHistoryMessage `json:"messages"`
		}{view, messages})
	}
	if err := printOperationHistory([]operationHistoryView{view}); err != nil {
		return err
	}
	for i := range data.Messages {
		message := &data.Messages[i]
		if _, err := fmt.Fprintf(os.Stdout, "%s %d %s %s\n", message.Timestamp.Format(time.RFC3339Nano), message.SequenceNumber, message.MessageType, message.MessageData); err != nil {
			return err
		}
	}
	return nil
}

func printOperationHistory(views []operationHistoryView) error {
	printer := output.NewWithColour(output.FormatTable, output.ColourSupported(flagNoColour))
	if len(views) == 0 {
		return printer.PrintEmpty(os.Stdout, "No recorded operations found.")
	}
	table := &output.TableData{Columns: []output.Column{
		{Header: "Operation ID", Field: "operation_id"},
		{Header: "Server ID", Field: "server_id"},
		{Header: "Server", Field: "server_name"},
		{Header: "Stack", Field: "stack_name"},
		{Header: "Command", Field: "command"},
		{Header: "Completion", Field: "completion"},
		{Header: "Outcome", Field: "outcome"},
		{Header: "Start", Field: "start_time"},
		{Header: "End", Field: "end_time"},
		{Header: "Exit Code", Field: "exit_code"},
		{Header: "Duration (ms)", Field: "duration_ms"},
	}, Rows: make([]map[string]any, len(views))}
	for i := range views {
		view := &views[i]
		row := map[string]any{
			"operation_id": view.OperationID, "server_id": view.ServerID, "server_name": view.ServerName,
			"stack_name": view.StackName, "command": view.Command, "completion": view.Completion,
			"outcome": "unknown", "start_time": "n/a", "end_time": "n/a", "exit_code": "n/a", "duration_ms": "n/a",
		}
		if view.Success != nil {
			if *view.Success {
				row["outcome"] = "success"
			} else {
				row["outcome"] = "failed"
			}
		}
		if view.StartTime != nil {
			row["start_time"] = view.StartTime.Format(time.RFC3339Nano)
		}
		if view.EndTime != nil {
			row["end_time"] = view.EndTime.Format(time.RFC3339Nano)
		}
		if view.ExitCode != nil {
			row["exit_code"] = *view.ExitCode
		}
		if view.DurationMs != nil {
			row["duration_ms"] = *view.DurationMs
		}
		table.Rows[i] = row
	}
	return printer.Print(os.Stdout, table)
}
