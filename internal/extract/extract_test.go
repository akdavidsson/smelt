package extract

import (
	"testing"
	"time"

	"github.com/akdavidsson/smelt/internal/capture"
	"github.com/akdavidsson/smelt/internal/schema"
)

// ── coerceValue ──────────────────────────────────────────────────────────────

func TestCoerceValue_String(t *testing.T) {
	v, w := coerceValue("hello", schema.TypeString, false)
	if v != "hello" || w != "" {
		t.Errorf("got (%v, %q)", v, w)
	}
}

func TestCoerceValue_Int(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
	}{
		{"42", 42},
		{"-7", -7},
		{"1,000,000", 1000000},
		{"  99  ", 99},
	}
	for _, c := range cases {
		v, w := coerceValue(c.raw, schema.TypeInt, false)
		if w != "" {
			t.Errorf("int(%q): unexpected warning %q", c.raw, w)
		}
		if v.(int64) != c.want {
			t.Errorf("int(%q) = %v, want %d", c.raw, v, c.want)
		}
	}
}

func TestCoerceValue_IntInvalid(t *testing.T) {
	v, w := coerceValue("not-a-number", schema.TypeInt, false)
	if w == "" {
		t.Error("want warning for invalid int")
	}
	if v != "not-a-number" {
		t.Errorf("non-nullable fallback should return raw string, got %v", v)
	}
}

func TestCoerceValue_IntInvalidNullable(t *testing.T) {
	v, w := coerceValue("bad", schema.TypeInt, true)
	if w == "" {
		t.Error("want warning")
	}
	if v != nil {
		t.Errorf("nullable fallback should return nil, got %v", v)
	}
}

func TestCoerceValue_Float(t *testing.T) {
	cases := []struct {
		raw  string
		want float64
	}{
		{"3.14", 3.14},
		{"-1.5", -1.5},
		{"1,234.56", 1234.56},
	}
	for _, c := range cases {
		v, w := coerceValue(c.raw, schema.TypeFloat, false)
		if w != "" {
			t.Errorf("float(%q): unexpected warning %q", c.raw, w)
		}
		if v.(float64) != c.want {
			t.Errorf("float(%q) = %v, want %f", c.raw, v, c.want)
		}
	}
}

func TestCoerceValue_Bool(t *testing.T) {
	trueInputs := []string{"true", "True", "TRUE", "yes", "YES", "1"}
	falseInputs := []string{"false", "False", "FALSE", "no", "NO", "0"}

	for _, raw := range trueInputs {
		v, w := coerceValue(raw, schema.TypeBool, false)
		if w != "" || v != true {
			t.Errorf("bool(%q): want true/no-warn, got %v %q", raw, v, w)
		}
	}
	for _, raw := range falseInputs {
		v, w := coerceValue(raw, schema.TypeBool, false)
		if w != "" || v != false {
			t.Errorf("bool(%q): want false/no-warn, got %v %q", raw, v, w)
		}
	}
}

func TestCoerceValue_BoolInvalid(t *testing.T) {
	_, w := coerceValue("maybe", schema.TypeBool, false)
	if w == "" {
		t.Error("want warning for invalid bool")
	}
}

func TestCoerceValue_Date(t *testing.T) {
	cases := []string{
		"2024-01-15",
		"01/15/2024",
		"Jan 15, 2024",
		"January 15, 2024",
		"15-Jan-2024",
	}
	for _, raw := range cases {
		v, w := coerceValue(raw, schema.TypeDate, false)
		if w != "" {
			t.Errorf("date(%q): unexpected warning %q", raw, w)
		}
		if _, ok := v.(time.Time); !ok {
			t.Errorf("date(%q): want time.Time, got %T", raw, v)
		}
	}
}

func TestCoerceValue_Datetime(t *testing.T) {
	cases := []string{
		"2024-01-15T10:30:00Z",
		"2024-01-15T10:30:00",
		"2024-01-15 10:30:00",
	}
	for _, raw := range cases {
		v, w := coerceValue(raw, schema.TypeDatetime, false)
		if w != "" {
			t.Errorf("datetime(%q): unexpected warning %q", raw, w)
		}
		if _, ok := v.(time.Time); !ok {
			t.Errorf("datetime(%q): want time.Time, got %T", raw, v)
		}
	}
}

func TestCoerceValue_EmptyNullable(t *testing.T) {
	v, w := coerceValue("", schema.TypeInt, true)
	if v != nil || w != "" {
		t.Errorf("empty nullable: want (nil, \"\"), got (%v, %q)", v, w)
	}
}

func TestCoerceValue_EmptyNonNullable(t *testing.T) {
	v, w := coerceValue("", schema.TypeString, false)
	if v != "" || w != "" {
		t.Errorf("empty non-nullable: want (\"\", \"\"), got (%v, %q)", v, w)
	}
}

// ── matchColumns ─────────────────────────────────────────────────────────────

func TestMatchColumns_ExactMatch(t *testing.T) {
	cols := []schema.Column{{Name: "country"}, {Name: "gdp"}}
	headers := []string{"country", "gdp"}
	m := matchColumns(cols, headers)
	if m["country"] != 0 || m["gdp"] != 1 {
		t.Errorf("exact match failed: %v", m)
	}
}

func TestMatchColumns_CaseInsensitive(t *testing.T) {
	cols := []schema.Column{{Name: "country"}}
	headers := []string{"Country"}
	m := matchColumns(cols, headers)
	if m["country"] != 0 {
		t.Errorf("case-insensitive match failed: %v", m)
	}
}

func TestMatchColumns_SpaceToUnderscore(t *testing.T) {
	cols := []schema.Column{{Name: "gdp_per_capita"}}
	headers := []string{"GDP Per Capita"}
	m := matchColumns(cols, headers)
	if m["gdp_per_capita"] != 0 {
		t.Errorf("space-to-underscore match failed: %v", m)
	}
}

func TestMatchColumns_NoMatch(t *testing.T) {
	cols := []schema.Column{{Name: "revenue"}}
	headers := []string{"country", "gdp"}
	m := matchColumns(cols, headers)
	if _, ok := m["revenue"]; ok {
		t.Error("should not match unrelated header")
	}
}

// ── Extract ───────────────────────────────────────────────────────────────────

func makeRegion(headers []string, rows [][]string) capture.Region {
	return capture.Region{
		Type:    capture.RegionTable,
		Headers: headers,
		Rows:    rows,
	}
}

func makeSchema(cols ...schema.Column) schema.Schema {
	return schema.Schema{Name: "test", Columns: cols}
}

func TestExtract_Basic(t *testing.T) {
	region := makeRegion(
		[]string{"country", "gdp"},
		[][]string{
			{"USA", "25"},
			{"China", "17"},
		},
	)
	s := makeSchema(
		schema.Column{Name: "country", Type: schema.TypeString},
		schema.Column{Name: "gdp", Type: schema.TypeInt},
	)

	result, err := Extract(region, s)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("want 2 records, got %d", len(result.Records))
	}
	if result.Records[0]["country"] != "USA" {
		t.Errorf("country: want USA, got %v", result.Records[0]["country"])
	}
	if result.Records[0]["gdp"] != int64(25) {
		t.Errorf("gdp: want 25, got %v", result.Records[0]["gdp"])
	}
}

func TestExtract_EmptyRegion(t *testing.T) {
	region := makeRegion([]string{"a"}, nil)
	s := makeSchema(schema.Column{Name: "a", Type: schema.TypeString})

	result, err := Extract(region, s)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(result.Records) != 0 {
		t.Errorf("want 0 records for empty region, got %d", len(result.Records))
	}
}

func TestExtract_WarningsOnCoercionFailure(t *testing.T) {
	region := makeRegion(
		[]string{"count"},
		[][]string{{"not-an-int"}, {"42"}},
	)
	s := makeSchema(schema.Column{Name: "count", Type: schema.TypeInt})

	result, err := Extract(region, s)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("want at least one warning for coercion failure")
	}
	// Second row should still extract correctly
	if result.Records[1]["count"] != int64(42) {
		t.Errorf("second row: want 42, got %v", result.Records[1]["count"])
	}
}

func TestExtract_PositionalFallback(t *testing.T) {
	// Headers don't match schema names → positional fallback
	region := makeRegion(
		[]string{"Column_A", "Column_B"},
		[][]string{{"hello", "99"}},
	)
	s := makeSchema(
		schema.Column{Name: "unrelated_x", Type: schema.TypeString},
		schema.Column{Name: "unrelated_y", Type: schema.TypeInt},
	)

	result, err := Extract(region, s)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("want 1 record, got %d", len(result.Records))
	}
	if result.Records[0]["unrelated_x"] != "hello" {
		t.Errorf("positional col 0: want hello, got %v", result.Records[0]["unrelated_x"])
	}
	if result.Records[0]["unrelated_y"] != int64(99) {
		t.Errorf("positional col 1: want 99, got %v", result.Records[0]["unrelated_y"])
	}
}

func TestExtract_NullableColumn(t *testing.T) {
	region := makeRegion(
		[]string{"name", "score"},
		[][]string{{"Alice", ""}, {"Bob", "95"}},
	)
	s := makeSchema(
		schema.Column{Name: "name", Type: schema.TypeString, Nullable: false},
		schema.Column{Name: "score", Type: schema.TypeInt, Nullable: true},
	)

	result, err := Extract(region, s)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if result.Records[0]["score"] != nil {
		t.Errorf("missing nullable value should be nil, got %v", result.Records[0]["score"])
	}
	if result.Records[1]["score"] != int64(95) {
		t.Errorf("present value should parse, got %v", result.Records[1]["score"])
	}
}
