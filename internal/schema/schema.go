package schema

import (
	"fmt"
	"strings"
)

// DataType represents the type of a column's data.
type DataType string

const (
	TypeString   DataType = "string"
	TypeInt      DataType = "int"
	TypeFloat    DataType = "float"
	TypeBool     DataType = "bool"
	TypeDate     DataType = "date"
	TypeDatetime DataType = "datetime"
)

// IsValidDataType returns true if dt is a recognized DataType.
func IsValidDataType(dt DataType) bool {
	switch dt {
	case TypeString, TypeInt, TypeFloat, TypeBool, TypeDate, TypeDatetime:
		return true
	}
	return false
}

// Column describes a single field in the schema.
type Column struct {
	Name        string   `json:"name"`
	Type        DataType `json:"type"`
	Nullable    bool     `json:"nullable"`
	Description string   `json:"description,omitempty"`
	Columns     []Column `json:"columns,omitempty"` // nested columns for hierarchical data
}

// Schema is the top-level IR returned by inference.
type Schema struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Columns     []Column `json:"columns"`
}

// Validate checks that the schema is internally consistent.
func (s *Schema) Validate() error {
	if len(s.Columns) == 0 {
		return fmt.Errorf("schema must have at least one column")
	}
	for i, col := range s.Columns {
		if err := validateColumn(col, i); err != nil {
			return err
		}
	}
	return nil
}

func validateColumn(col Column, idx int) error {
	if col.Name == "" {
		return fmt.Errorf("column[%d] has empty name", idx)
	}
	if !IsValidDataType(col.Type) {
		return fmt.Errorf("column %q has invalid type %q", col.Name, col.Type)
	}
	for i, sub := range col.Columns {
		if err := validateColumn(sub, i); err != nil {
			return fmt.Errorf("column %q: %w", col.Name, err)
		}
	}
	return nil
}

// FlatColumns returns a flat list of column names, recursing into nested columns.
func (s *Schema) FlatColumns() []string {
	return flatColumns(s.Columns, "")
}

func flatColumns(cols []Column, prefix string) []string {
	var names []string
	for _, col := range cols {
		name := col.Name
		if prefix != "" {
			name = prefix + "." + name
		}
		if len(col.Columns) > 0 {
			names = append(names, flatColumns(col.Columns, name)...)
		} else {
			names = append(names, name)
		}
	}
	return names
}

// NormalizeName converts a column name to a comparable form (lowercase, underscores).
func NormalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}
