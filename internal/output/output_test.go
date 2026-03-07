package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/arnordavidsson/smelt/internal/extract"
	"github.com/arnordavidsson/smelt/internal/schema"
)

func makeResult(cols []schema.Column, records []extract.Record) *extract.Result {
	return &extract.Result{
		Schema:  schema.Schema{Name: "test", Columns: cols},
		Records: records,
	}
}

// ── WriteJSON ─────────────────────────────────────────────────────────────────

func TestWriteJSON_Basic(t *testing.T) {
	result := makeResult(
		[]schema.Column{{Name: "name", Type: schema.TypeString}},
		[]extract.Record{
			{"name": "Alice"},
			{"name": "Bob"},
		},
	)

	var buf bytes.Buffer
	if err := WriteJSON(&buf, result); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	var records []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &records); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(records) != 2 {
		t.Errorf("want 2 records, got %d", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Errorf("first record: want Alice, got %v", records[0]["name"])
	}
}

func TestWriteJSON_Empty(t *testing.T) {
	result := makeResult(nil, nil)
	var buf bytes.Buffer
	if err := WriteJSON(&buf, result); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}
	// Should produce "null\n" or "[]"
	trimmed := strings.TrimSpace(buf.String())
	if trimmed != "null" && trimmed != "[]" {
		t.Errorf("unexpected empty output: %q", trimmed)
	}
}

func TestWriteJSON_NullValues(t *testing.T) {
	result := makeResult(
		[]schema.Column{{Name: "val", Type: schema.TypeInt, Nullable: true}},
		[]extract.Record{{"val": nil}},
	)
	var buf bytes.Buffer
	if err := WriteJSON(&buf, result); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}
	if !strings.Contains(buf.String(), "null") {
		t.Errorf("expected null in output: %s", buf.String())
	}
}

// ── WriteAllJSON ──────────────────────────────────────────────────────────────

func TestWriteAllJSON_Basic(t *testing.T) {
	tables := []TableResult{
		{Name: "gdp", Context: "GDP Table", Records: []extract.Record{{"country": "USA"}}},
		{Name: "pop", Records: []extract.Record{{"country": "China"}}},
	}

	var buf bytes.Buffer
	if err := WriteAllJSON(&buf, tables); err != nil {
		t.Fatalf("WriteAllJSON error: %v", err)
	}

	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(out) != 2 {
		t.Fatalf("want 2 table results, got %d", len(out))
	}
	if out[0]["name"] != "gdp" {
		t.Errorf("first table name: want gdp, got %v", out[0]["name"])
	}
	if out[0]["context"] != "GDP Table" {
		t.Errorf("first table context: want GDP Table, got %v", out[0]["context"])
	}
	// Empty context should be omitted (omitempty)
	if _, ok := out[1]["context"]; ok {
		t.Error("empty context should be omitted from JSON")
	}
}

func TestWriteAllJSON_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteAllJSON(&buf, nil); err != nil {
		t.Fatalf("WriteAllJSON error: %v", err)
	}
	trimmed := strings.TrimSpace(buf.String())
	if trimmed != "null" && trimmed != "[]" {
		t.Errorf("unexpected output for empty slice: %q", trimmed)
	}
}

// ── WriteCSV ──────────────────────────────────────────────────────────────────

func TestWriteCSV_Basic(t *testing.T) {
	result := makeResult(
		[]schema.Column{
			{Name: "country", Type: schema.TypeString},
			{Name: "gdp", Type: schema.TypeInt},
		},
		[]extract.Record{
			{"country": "USA", "gdp": int64(25)},
			{"country": "China", "gdp": int64(17)},
		},
	)

	var buf bytes.Buffer
	if err := WriteCSV(&buf, result); err != nil {
		t.Fatalf("WriteCSV error: %v", err)
	}

	// Split on \n (csv.Writer default); last element is empty trailing newline
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 { // header + 2 data rows
		t.Fatalf("want 3 lines (header + 2 rows), got %d:\n%s", len(lines), buf.String())
	}
	if lines[0] != "country,gdp" {
		t.Errorf("header: want country,gdp got %q", lines[0])
	}
	if !strings.Contains(lines[1], "USA") {
		t.Errorf("first row should contain USA: %q", lines[1])
	}
}

func TestWriteCSV_NullValue(t *testing.T) {
	result := makeResult(
		[]schema.Column{{Name: "val", Type: schema.TypeInt, Nullable: true}},
		[]extract.Record{{"val": nil}},
	)
	var buf bytes.Buffer
	if err := WriteCSV(&buf, result); err != nil {
		t.Fatalf("WriteCSV error: %v", err)
	}
	lines := strings.Split(buf.String(), "\n")
	// lines: ["val", "", ""] (header, empty data row, trailing newline)
	if len(lines) < 2 {
		t.Fatalf("want at least 2 lines, got %d: %q", len(lines), buf.String())
	}
	if lines[1] != "" {
		t.Errorf("nil value should produce empty CSV cell, got %q", lines[1])
	}
}

func TestWriteCSV_TypeFormatting(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := makeResult(
		[]schema.Column{
			{Name: "flag", Type: schema.TypeBool},
			{Name: "amount", Type: schema.TypeFloat},
			{Name: "ts", Type: schema.TypeDatetime},
		},
		[]extract.Record{{"flag": true, "amount": 3.14, "ts": ts}},
	)
	var buf bytes.Buffer
	if err := WriteCSV(&buf, result); err != nil {
		t.Fatalf("WriteCSV error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "true") {
		t.Errorf("bool true not formatted correctly: %q", out)
	}
	if !strings.Contains(out, "3.14") {
		t.Errorf("float not formatted correctly: %q", out)
	}
	if !strings.Contains(out, "2024-01-15") {
		t.Errorf("datetime not formatted correctly: %q", out)
	}
}

// ── formatCSVValue ────────────────────────────────────────────────────────────

func TestFormatCSVValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{int64(42), "42"},
		{float64(1.5), "1.5"},
		{time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "2024-06-01T00:00:00Z"},
	}
	for _, c := range cases {
		if got := formatCSVValue(c.in); got != c.want {
			t.Errorf("formatCSVValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
