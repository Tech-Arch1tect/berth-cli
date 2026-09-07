package output

import (
	"encoding/json"
	"fmt"
	"io"
)

type JSONPrinter struct{}

func (p *JSONPrinter) Print(w io.Writer, data any) error {
	if tableData, ok := data.(*TableData); ok {
		data = tableData.Rows
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (p *JSONPrinter) PrintEmpty(w io.Writer, message string) error {
	_, err := fmt.Fprintln(w, "[]")
	return err
}
