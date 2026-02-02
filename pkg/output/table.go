package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type TablePrinter struct{}

func (p *TablePrinter) Print(w io.Writer, data any) error {
	tableData, ok := data.(*TableData)
	if !ok {
		return fmt.Errorf("table printer requires *TableData, got %T", data)
	}

	if len(tableData.Rows) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	headers := make([]string, len(tableData.Columns))
	separators := make([]string, len(tableData.Columns))
	for i, col := range tableData.Columns {
		headers[i] = col.Header
		separators[i] = strings.Repeat("-", len(col.Header))
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	fmt.Fprintln(tw, strings.Join(separators, "\t"))

	for _, row := range tableData.Rows {
		values := make([]string, len(tableData.Columns))
		for i, col := range tableData.Columns {
			values[i] = formatValue(row[col.Field])
		}
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	return tw.Flush()
}

func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case bool:
		if val {
			return "Yes"
		}
		return "No"
	default:
		return fmt.Sprintf("%v", val)
	}
}
