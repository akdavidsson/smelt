package capture

import (
	"strings"
	"testing"
)

func TestDetectRegions_Empty(t *testing.T) {
	if detectRegions("", 1) != nil {
		t.Error("want nil for empty text")
	}
	if detectRegions("   \n\n  ", 1) != nil {
		t.Error("want nil for blank text")
	}
}

func TestDetectRegions_AlignedTable(t *testing.T) {
	// 2+ space gaps simulate pdftotext -layout column alignment
	text := `Country       GDP     Growth
USA           25.5    2.1%
China         17.7    5.2%
Germany       4.1     -0.3%`

	regions := detectRegions(text, 1)
	if len(regions) == 0 {
		t.Fatal("want at least one region")
	}
	r := regions[0]
	if r.Type != RegionTable {
		t.Errorf("want RegionTable, got %q", r.Type)
	}
	if r.Headers[0] != "Country" {
		t.Errorf("first header: want Country, got %q", r.Headers[0])
	}
	if len(r.Rows) < 3 {
		t.Errorf("want at least 3 rows, got %d", len(r.Rows))
	}
	if r.SourcePage != 1 {
		t.Errorf("source page: want 1, got %d", r.SourcePage)
	}
}

func TestDetectRegions_SparseTextFallback(t *testing.T) {
	// Only single-token lines → no table structure
	text := `Introduction
This document covers GDP data.
See table below.`

	regions := detectRegions(text, 2)
	if len(regions) == 0 {
		t.Fatal("want a text region")
	}
	if regions[0].Type != RegionText {
		t.Errorf("want RegionText, got %q", regions[0].Type)
	}
	if regions[0].SourcePage != 2 {
		t.Errorf("source page: want 2, got %d", regions[0].SourcePage)
	}
}

func TestDetectRegions_PageNumber(t *testing.T) {
	text := `Col1  Col2
A     1
B     2
C     3`
	regions := detectRegions(text, 7)
	if len(regions) == 0 || regions[0].SourcePage != 7 {
		t.Error("source page should match argument")
	}
}

func TestParseContentStream_TjOperator(t *testing.T) {
	stream := `(Hello) Tj (World) Tj`
	got := parseContentStream(stream)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Errorf("expected Hello and World in output, got %q", got)
	}
}

func TestParseContentStream_TJArray(t *testing.T) {
	stream := `[(GDP) 20 ( Data)] TJ`
	got := parseContentStream(stream)
	if !strings.Contains(got, "GDP") || !strings.Contains(got, " Data") {
		t.Errorf("expected GDP and Data in output, got %q", got)
	}
}

func TestParseContentStream_EscapedParens(t *testing.T) {
	stream := `(Hello \(World\)) Tj`
	got := parseContentStream(stream)
	if !strings.Contains(got, "Hello (World)") {
		t.Errorf("escaped parens not handled: %q", got)
	}
}

func TestParseContentStream_Empty(t *testing.T) {
	if got := parseContentStream(""); got != "" {
		t.Errorf("want empty string, got %q", got)
	}
	if got := parseContentStream("/F1 12 Tf 100 200 Td"); got != "" {
		t.Errorf("non-text operators should produce no output, got %q", got)
	}
}

func TestUnescapePDF(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`hello\nworld`, "hello\nworld"},
		{`tab\there`, "tab\there"},
		{`back\\slash`, `back\slash`},
		{`open\(paren`, "open(paren"},
		{`close\)paren`, "close)paren"},
	}
	for _, c := range cases {
		if got := unescapePDF(c.in); got != c.want {
			t.Errorf("unescapePDF(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
