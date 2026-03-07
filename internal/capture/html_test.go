package capture

import (
	"strings"
	"testing"
)

func captureHTML(t *testing.T, html string) []Region {
	t.Helper()
	regions, err := (&HTMLCapturer{}).Capture([]byte(html))
	if err != nil {
		t.Fatalf("Capture error: %v", err)
	}
	return regions
}

func TestHTMLCapturer_TheadTh(t *testing.T) {
	html := `<table>
		<thead><tr><th>Country</th><th>GDP</th></tr></thead>
		<tbody>
			<tr><td>USA</td><td>25.5</td></tr>
			<tr><td>China</td><td>17.7</td></tr>
		</tbody>
	</table>`

	regions := captureHTML(t, html)
	if len(regions) != 1 {
		t.Fatalf("want 1 region, got %d", len(regions))
	}
	r := regions[0]
	if r.Type != RegionTable {
		t.Errorf("want RegionTable, got %q", r.Type)
	}
	if got := strings.Join(r.Headers, ","); got != "Country,GDP" {
		t.Errorf("headers: want Country,GDP got %q", got)
	}
	if len(r.Rows) != 2 {
		t.Errorf("want 2 rows, got %d", len(r.Rows))
	}
	if r.Rows[0][0] != "USA" || r.Rows[0][1] != "25.5" {
		t.Errorf("row[0] unexpected: %v", r.Rows[0])
	}
}

func TestHTMLCapturer_FallbackThInFirstRow(t *testing.T) {
	html := `<table>
		<tr><th>Name</th><th>Value</th></tr>
		<tr><td>A</td><td>1</td></tr>
		<tr><td>B</td><td>2</td></tr>
	</table>`

	regions := captureHTML(t, html)
	if len(regions) != 1 {
		t.Fatalf("want 1 region, got %d", len(regions))
	}
	if got := strings.Join(regions[0].Headers, ","); got != "Name,Value" {
		t.Errorf("headers: want Name,Value got %q", got)
	}
	if len(regions[0].Rows) != 2 {
		t.Errorf("want 2 data rows, got %d", len(regions[0].Rows))
	}
}

func TestHTMLCapturer_FallbackFirstTdAsHeaders(t *testing.T) {
	html := `<table>
		<tr><td>Col1</td><td>Col2</td></tr>
		<tr><td>X</td><td>1</td></tr>
		<tr><td>Y</td><td>2</td></tr>
	</table>`

	regions := captureHTML(t, html)
	if len(regions) != 1 {
		t.Fatalf("want 1 region, got %d", len(regions))
	}
	if got := strings.Join(regions[0].Headers, ","); got != "Col1,Col2" {
		t.Errorf("headers: %q", got)
	}
	// First row was consumed as headers, only 2 data rows remain
	if len(regions[0].Rows) != 2 {
		t.Errorf("want 2 rows, got %d", len(regions[0].Rows))
	}
}

func TestHTMLCapturer_CaptionContext(t *testing.T) {
	html := `<table>
		<caption>Annual Revenue</caption>
		<thead><tr><th>Year</th><th>Revenue</th></tr></thead>
		<tbody><tr><td>2023</td><td>100</td></tr></tbody>
	</table>`

	regions := captureHTML(t, html)
	if len(regions) != 1 {
		t.Fatalf("want 1 region, got %d", len(regions))
	}
	if regions[0].Context != "Annual Revenue" {
		t.Errorf("context: want %q got %q", "Annual Revenue", regions[0].Context)
	}
}

func TestHTMLCapturer_HeadingContext(t *testing.T) {
	html := `<h2>GDP Data</h2>
	<table>
		<thead><tr><th>Country</th><th>GDP</th></tr></thead>
		<tbody><tr><td>USA</td><td>25</td></tr></tbody>
	</table>`

	regions := captureHTML(t, html)
	if len(regions) != 1 {
		t.Fatalf("want 1 region, got %d", len(regions))
	}
	if regions[0].Context != "GDP Data" {
		t.Errorf("context: want %q got %q", "GDP Data", regions[0].Context)
	}
}

func TestHTMLCapturer_MultipleTables(t *testing.T) {
	html := `
	<table>
		<thead><tr><th>A</th><th>B</th></tr></thead>
		<tbody><tr><td>1</td><td>2</td></tr></tbody>
	</table>
	<table>
		<thead><tr><th>X</th><th>Y</th><th>Z</th></tr></thead>
		<tbody>
			<tr><td>a</td><td>b</td><td>c</td></tr>
			<tr><td>d</td><td>e</td><td>f</td></tr>
		</tbody>
	</table>`

	regions := captureHTML(t, html)
	if len(regions) != 2 {
		t.Fatalf("want 2 regions, got %d", len(regions))
	}
	if len(regions[0].Headers) != 2 {
		t.Errorf("table1: want 2 headers, got %d", len(regions[0].Headers))
	}
	if len(regions[1].Headers) != 3 {
		t.Errorf("table2: want 3 headers, got %d", len(regions[1].Headers))
	}
}

func TestHTMLCapturer_EmptyTable(t *testing.T) {
	html := `<table></table>`
	regions := captureHTML(t, html)
	if len(regions) != 0 {
		t.Errorf("want 0 regions for empty table, got %d", len(regions))
	}
}

func TestHTMLCapturer_WhitespaceCollapsed(t *testing.T) {
	html := `<table>
		<thead><tr><th>  Col  One  </th></tr></thead>
		<tbody><tr><td>  val  </td></tr></tbody>
	</table>`

	regions := captureHTML(t, html)
	if len(regions) != 1 {
		t.Fatalf("want 1 region, got %d", len(regions))
	}
	if regions[0].Headers[0] != "Col One" {
		t.Errorf("header not trimmed: %q", regions[0].Headers[0])
	}
	if regions[0].Rows[0][0] != "val" {
		t.Errorf("cell not trimmed: %q", regions[0].Rows[0][0])
	}
}

func TestHTMLCapturer_RawTextPopulated(t *testing.T) {
	html := `<table>
		<thead><tr><th>A</th><th>B</th></tr></thead>
		<tbody><tr><td>1</td><td>2</td></tr></tbody>
	</table>`

	regions := captureHTML(t, html)
	if regions[0].RawText == "" {
		t.Error("RawText should not be empty")
	}
	if !strings.Contains(regions[0].RawText, "A") {
		t.Error("RawText should contain header content")
	}
}

func TestCleanText(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  hello  ", "hello"},
		{"a  b  c", "a b c"},
		{"\t\n foo \n\t", "foo"},
		{"", ""},
	}
	for _, c := range cases {
		if got := cleanText(c.in); got != c.want {
			t.Errorf("cleanText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
