package output

import (
	"encoding/json"
	"io"

	"github.com/arnordavidsson/smelt/internal/extract"
)

// WriteJSON writes the extraction result as indented JSON.
func WriteJSON(w io.Writer, result *extract.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result.Records)
}

// TableResult holds extracted data for one table, used by WriteAllJSON.
type TableResult struct {
	Name    string          `json:"name"`
	Context string          `json:"context,omitempty"`
	Records []extract.Record `json:"records"`
}

// WriteAllJSON writes multiple table results as an indented JSON array.
func WriteAllJSON(w io.Writer, tables []TableResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(tables)
}
