package output

import (
	"fmt"
	"io"
	"os"
)

type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
)

func ParseFormat(s string) (Format, error) {
	switch s {
	case "table", "":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("invalid output format %q (valid: table, json)", s)
	}
}

type Printer interface {
	Print(w io.Writer, data any) error
	PrintEmpty(w io.Writer, message string) error
}

func New(format Format) Printer {
	switch format {
	case FormatJSON:
		return &JSONPrinter{}
	default:
		return &TablePrinter{}
	}
}

func NewWithColour(format Format, colourEnabled bool) Printer {
	printer := New(format)
	if tablePrinter, ok := printer.(*TablePrinter); ok {
		tablePrinter.colourEnabled = colourEnabled
	}
	return printer
}

func Print(format Format, data any) error {
	return New(format).Print(os.Stdout, data)
}

type Column struct {
	Header string
	Field  string
	Colour ColourFunc
}

type TableData struct {
	Columns []Column
	Rows    []map[string]any
}
