package schema

import (
	"testing"
)

func validSchema() Schema {
	return Schema{
		Name:        "test",
		Description: "A test schema",
		Columns: []Column{
			{Name: "id", Type: TypeInt, Nullable: false},
			{Name: "name", Type: TypeString, Nullable: false},
			{Name: "score", Type: TypeFloat, Nullable: true},
		},
	}
}

func TestSchema_Validate_Valid(t *testing.T) {
	s := validSchema()
	if err := s.Validate(); err != nil {
		t.Errorf("valid schema should not error: %v", err)
	}
}

func TestSchema_Validate_NoColumns(t *testing.T) {
	s := Schema{Name: "empty"}
	if err := s.Validate(); err == nil {
		t.Error("want error for schema with no columns")
	}
}

func TestSchema_Validate_EmptyColumnName(t *testing.T) {
	s := Schema{
		Name:    "test",
		Columns: []Column{{Name: "", Type: TypeString}},
	}
	if err := s.Validate(); err == nil {
		t.Error("want error for empty column name")
	}
}

func TestSchema_Validate_InvalidType(t *testing.T) {
	s := Schema{
		Name:    "test",
		Columns: []Column{{Name: "col", Type: "invalid_type"}},
	}
	if err := s.Validate(); err == nil {
		t.Error("want error for invalid column type")
	}
}

func TestSchema_Validate_AllTypes(t *testing.T) {
	types := []DataType{TypeString, TypeInt, TypeFloat, TypeBool, TypeDate, TypeDatetime}
	for _, dt := range types {
		s := Schema{
			Name:    "test",
			Columns: []Column{{Name: "col", Type: dt}},
		}
		if err := s.Validate(); err != nil {
			t.Errorf("type %q should be valid: %v", dt, err)
		}
	}
}

func TestSchema_Validate_NestedColumns(t *testing.T) {
	s := Schema{
		Name: "test",
		Columns: []Column{
			{
				Name: "parent",
				Type: TypeString,
				Columns: []Column{
					{Name: "child", Type: TypeInt},
				},
			},
		},
	}
	if err := s.Validate(); err != nil {
		t.Errorf("nested columns should be valid: %v", err)
	}
}

func TestSchema_Validate_InvalidNestedColumn(t *testing.T) {
	s := Schema{
		Name: "test",
		Columns: []Column{
			{
				Name: "parent",
				Type: TypeString,
				Columns: []Column{
					{Name: "", Type: TypeInt}, // invalid: empty name
				},
			},
		},
	}
	if err := s.Validate(); err == nil {
		t.Error("want error for invalid nested column")
	}
}

func TestFlatColumns_Flat(t *testing.T) {
	s := validSchema()
	cols := s.FlatColumns()
	if len(cols) != 3 {
		t.Fatalf("want 3 columns, got %d", len(cols))
	}
	if cols[0] != "id" || cols[1] != "name" || cols[2] != "score" {
		t.Errorf("unexpected columns: %v", cols)
	}
}

func TestFlatColumns_Nested(t *testing.T) {
	s := Schema{
		Name: "test",
		Columns: []Column{
			{Name: "top", Type: TypeString},
			{
				Name: "nested",
				Type: TypeString,
				Columns: []Column{
					{Name: "a", Type: TypeInt},
					{Name: "b", Type: TypeFloat},
				},
			},
		},
	}
	cols := s.FlatColumns()
	// "top", "nested.a", "nested.b"
	if len(cols) != 3 {
		t.Fatalf("want 3 flat columns, got %d: %v", len(cols), cols)
	}
	if cols[1] != "nested.a" || cols[2] != "nested.b" {
		t.Errorf("nested column names wrong: %v", cols)
	}
}

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Country", "country"},
		{"GDP (USD)", "gdp_(usd)"},
		{"growth rate", "growth_rate"},
		{"Per-Capita Income", "per_capita_income"},
		{"already_snake", "already_snake"},
		{"UPPER CASE", "upper_case"},
	}
	for _, c := range cases {
		if got := NormalizeName(c.in); got != c.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsValidDataType(t *testing.T) {
	valid := []DataType{TypeString, TypeInt, TypeFloat, TypeBool, TypeDate, TypeDatetime}
	for _, dt := range valid {
		if !IsValidDataType(dt) {
			t.Errorf("%q should be valid", dt)
		}
	}
	if IsValidDataType("unknown") {
		t.Error("\"unknown\" should not be valid")
	}
	if IsValidDataType("") {
		t.Error("empty string should not be valid")
	}
}
