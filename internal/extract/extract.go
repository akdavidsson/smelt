package extract

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/akdavidsson/smelt/internal/capture"
	"github.com/akdavidsson/smelt/internal/schema"
)

// Record is a single extracted data row.
type Record = map[string]any

// Result holds the extraction output.
type Result struct {
	Schema   schema.Schema
	Records  []Record
	Warnings []string
}

// Extract maps a captured region onto a schema and returns structured records.
func Extract(region capture.Region, s schema.Schema) (*Result, error) {
	if len(region.Rows) == 0 {
		return &Result{Schema: s}, nil
	}

	colMap := matchColumns(s.Columns, region.Headers)
	result := &Result{Schema: s}

	for rowIdx, row := range region.Rows {
		rec := make(Record, len(s.Columns))
		for colIdx, col := range s.Columns {
			var raw string
			if srcIdx, ok := colMap[col.Name]; ok && srcIdx < len(row) {
				raw = strings.TrimSpace(row[srcIdx])
			} else if colIdx < len(row) {
				// positional fallback
				raw = strings.TrimSpace(row[colIdx])
			}

			val, warn := coerceValue(raw, col.Type, col.Nullable)
			if warn != "" {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("row %d, col %q: %s", rowIdx+1, col.Name, warn))
			}
			rec[col.Name] = val
		}
		result.Records = append(result.Records, rec)
	}

	return result, nil
}

// matchColumns returns a map of schema column name → source column index.
// Uses case-insensitive + underscore/space normalization, then positional fallback.
func matchColumns(schemaCols []schema.Column, headers []string) map[string]int {
	m := make(map[string]int)

	normalizedHeaders := make([]string, len(headers))
	for i, h := range headers {
		normalizedHeaders[i] = schema.NormalizeName(h)
	}

	for _, col := range schemaCols {
		normCol := schema.NormalizeName(col.Name)
		for i, nh := range normalizedHeaders {
			if nh == normCol {
				m[col.Name] = i
				break
			}
		}
	}

	return m
}

var dateFmts = []string{
	"2006-01-02",
	"01/02/2006",
	"Jan 2, 2006",
	"January 2, 2006",
	"02-Jan-2006",
}

var datetimeFmts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
}

// coerceValue attempts to parse raw into the given DataType.
// Returns (value, warningMessage). On soft failure returns (nil or rawString, warning).
func coerceValue(raw string, colType schema.DataType, nullable bool) (any, string) {
	if raw == "" {
		if nullable {
			return nil, ""
		}
		return raw, ""
	}

	switch colType {
	case schema.TypeString:
		return raw, ""

	case schema.TypeInt:
		cleaned := strings.ReplaceAll(raw, ",", "")
		v, err := strconv.ParseInt(strings.TrimSpace(cleaned), 10, 64)
		if err != nil {
			return softFail(raw, nullable, fmt.Sprintf("cannot parse %q as int", raw))
		}
		return v, ""

	case schema.TypeFloat:
		cleaned := strings.ReplaceAll(raw, ",", "")
		v, err := strconv.ParseFloat(strings.TrimSpace(cleaned), 64)
		if err != nil {
			return softFail(raw, nullable, fmt.Sprintf("cannot parse %q as float", raw))
		}
		return v, ""

	case schema.TypeBool:
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "yes", "1":
			return true, ""
		case "false", "no", "0":
			return false, ""
		}
		return softFail(raw, nullable, fmt.Sprintf("cannot parse %q as bool", raw))

	case schema.TypeDate:
		for _, fmt := range dateFmts {
			if t, err := time.Parse(fmt, strings.TrimSpace(raw)); err == nil {
				return t, ""
			}
		}
		return softFail(raw, nullable, fmt.Sprintf("cannot parse %q as date", raw))

	case schema.TypeDatetime:
		for _, fmt := range datetimeFmts {
			if t, err := time.Parse(fmt, strings.TrimSpace(raw)); err == nil {
				return t, ""
			}
		}
		return softFail(raw, nullable, fmt.Sprintf("cannot parse %q as datetime", raw))
	}

	return raw, ""
}

// softFail returns nil + warning if nullable, else raw + warning.
func softFail(raw string, nullable bool, warn string) (any, string) {
	if nullable {
		return nil, warn
	}
	return raw, warn
}
