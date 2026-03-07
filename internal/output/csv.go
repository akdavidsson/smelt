package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/akdavidsson/smelt/internal/extract"
)

// WriteCSV writes the extraction result as CSV with a header row.
func WriteCSV(w io.Writer, result *extract.Result) error {
	cw := csv.NewWriter(w)

	cols := result.Schema.FlatColumns()
	if err := cw.Write(cols); err != nil {
		return err
	}

	for _, rec := range result.Records {
		row := make([]string, len(cols))
		for i, col := range cols {
			row[i] = formatCSVValue(rec[col])
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// formatCSVValue converts a Go value to its CSV string representation.
func formatCSVValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", v)
	}
}
