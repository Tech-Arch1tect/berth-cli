package output

import (
	"encoding/json"
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
