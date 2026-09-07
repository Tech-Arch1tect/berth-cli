package output

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type TablePrinter struct {
	colourEnabled bool
}

type tableCell struct {
	text  string
	width int
}

func (p *TablePrinter) Print(w io.Writer, data any) error {
	tableData, ok := data.(*TableData)
	if !ok {
		return fmt.Errorf("table printer requires *TableData, got %T", data)
	}

	if len(tableData.Rows) == 0 {
		return nil
	}

	widths := make([]int, len(tableData.Columns))
	lines := make([][]tableCell, 0, len(tableData.Rows)+2)

	headers := make([]tableCell, len(tableData.Columns))
	for i, col := range tableData.Columns {
		headers[i] = plainCell(col.Header)
		widths[i] = headers[i].width
	}
	lines = append(lines, headers)

	separators := make([]tableCell, len(tableData.Columns))
	for i, col := range tableData.Columns {
		separators[i] = plainCell(strings.Repeat("-", len(col.Header)))
	}
	lines = append(lines, separators)

	for _, row := range tableData.Rows {
		cells := make([]tableCell, len(tableData.Columns))
		for i, col := range tableData.Columns {
			text := formatValue(row[col.Field])
			cells[i] = plainCell(text)
			widths[i] = max(widths[i], cells[i].width)
			if col.Colour != nil {
				if code, ok := col.Colour(row[col.Field]); ok {
					cells[i].text = colourise(text, code, p.colourEnabled)
				}
			}
		}
		lines = append(lines, cells)
	}

	var b strings.Builder
	for _, line := range lines {
		for i, cell := range line {
			b.WriteString(cell.text)
			if i < len(line)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-cell.width+2))
			}
		}
		b.WriteString("\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func (p *TablePrinter) PrintEmpty(w io.Writer, message string) error {
	_, err := fmt.Fprintln(w, message)
	return err
}

func plainCell(text string) tableCell {
	return tableCell{text: text, width: utf8.RuneCountInString(text)}
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
