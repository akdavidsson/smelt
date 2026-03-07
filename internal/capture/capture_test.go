package capture

import (
	"strings"
	"testing"
)

func makeTable(rows int, headers []string, context string) Region {
	r := Region{
		Type:    RegionTable,
		Headers: headers,
		Context: context,
	}
	for i := 0; i < rows; i++ {
		row := make([]string, len(headers))
		for j := range row {
			row[j] = "v"
		}
		r.Rows = append(r.Rows, row)
	}
	return r
}

func TestSelectBestRegion_Empty(t *testing.T) {
	if SelectBestRegion(nil, "") != nil {
		t.Error("want nil for empty regions")
	}
}

func TestSelectBestRegion_Single(t *testing.T) {
	r := makeTable(5, []string{"A", "B"}, "")
	regions := []Region{r}
	got := SelectBestRegion(regions, "")
	if got == nil {
		t.Fatal("want non-nil")
	}
	if len(got.Rows) != 5 {
		t.Errorf("got wrong region")
	}
}

func TestSelectBestRegion_PicksLargest(t *testing.T) {
	small := makeTable(3, []string{"A"}, "")
	large := makeTable(20, []string{"A", "B", "C"}, "")
	regions := []Region{small, large}

	got := SelectBestRegion(regions, "")
	if len(got.Rows) != 20 {
		t.Errorf("want largest table (20 rows), got %d rows", len(got.Rows))
	}
}

func TestSelectBestRegion_QueryBoostChangesWinner(t *testing.T) {
	// big: 4 rows × 10 + 1 col × 2 + 5 (context) = 47
	// small: 2 rows × 10 + 2 cols × 2 + 5 (context) = 29, but query adds +40 → 69
	big := makeTable(4, []string{"X"}, "Unrelated Stats")
	small := makeTable(2, []string{"Revenue", "Region"}, "Annual Revenue by Region")

	// Without query: big wins
	got := SelectBestRegion([]Region{big, small}, "")
	if len(got.Rows) != 4 {
		t.Errorf("without query: want big table (4 rows), got %d rows", len(got.Rows))
	}

	// With matching query: small table gets boosted and wins
	got = SelectBestRegion([]Region{big, small}, "revenue region")
	if got.Context != "Annual Revenue by Region" {
		t.Errorf("with query: want small table boosted, got context %q", got.Context)
	}
}

func TestSelectBestRegion_QueryMatchesHeaders(t *testing.T) {
	a := makeTable(5, []string{"Country", "Population"}, "")
	b := makeTable(5, []string{"Product", "Sales"}, "")

	got := SelectBestRegion([]Region{a, b}, "sales product")
	if got.Headers[0] != "Product" {
		t.Errorf("want header-matched table, got %q", got.Headers[0])
	}
}

func TestQueryBoost_NoTerms(t *testing.T) {
	r := makeTable(5, []string{"A"}, "Revenue")
	if queryBoost(&r, nil) != 0 {
		t.Error("want 0 boost with no terms")
	}
}

func TestQueryBoost_ContextMatch(t *testing.T) {
	r := makeTable(1, []string{"x"}, "GDP per Capita")
	boost := queryBoost(&r, []string{"gdp", "capita"})
	if boost <= 0 {
		t.Errorf("want positive boost, got %d", boost)
	}
}

func TestScoreRegion_TableVsText(t *testing.T) {
	table := makeTable(10, []string{"A", "B"}, "")
	text := Region{Type: RegionText, RawText: strings.Repeat("x", 5000)}

	tableScore := scoreRegion(&table)
	textScore := scoreRegion(&text)
	if tableScore <= textScore {
		t.Errorf("table (%d) should score higher than text (%d)", tableScore, textScore)
	}
}

func TestScoreRegion_ContextBonus(t *testing.T) {
	withCtx := makeTable(5, []string{"A"}, "My Table")
	withoutCtx := makeTable(5, []string{"A"}, "")

	if scoreRegion(&withCtx) <= scoreRegion(&withoutCtx) {
		t.Error("table with context should score higher")
	}
}

func TestFormatRegion_Table(t *testing.T) {
	r := Region{
		Type:    RegionTable,
		Headers: []string{"Country", "GDP"},
		Rows: [][]string{
			{"USA", "25.5"},
			{"China", "17.7"},
		},
		Context: "GDP Data",
	}
	out := FormatRegion(r)
	if !strings.Contains(out, "Country") {
		t.Error("output should contain headers")
	}
	if !strings.Contains(out, "USA") {
		t.Error("output should contain data rows")
	}
	if !strings.Contains(out, "GDP Data") {
		t.Error("output should contain context")
	}
}

func TestFormatRegion_Text(t *testing.T) {
	r := Region{Type: RegionText, RawText: "some raw text"}
	if !strings.Contains(FormatRegion(r), "some raw text") {
		t.Error("text region should output RawText")
	}
}

func TestFormatRegionSummary(t *testing.T) {
	table := makeTable(5, []string{"A", "B", "C"}, "My Table")
	summary := FormatRegionSummary(table)
	if !strings.Contains(summary, "3") {
		t.Errorf("summary should mention column count: %q", summary)
	}
	if !strings.Contains(summary, "5") {
		t.Errorf("summary should mention row count: %q", summary)
	}
	if !strings.Contains(summary, "My Table") {
		t.Errorf("summary should mention context: %q", summary)
	}

	text := Region{Type: RegionText, RawText: "hello"}
	if !strings.Contains(FormatRegionSummary(text), "text") {
		t.Errorf("text summary should say text: %q", FormatRegionSummary(text))
	}
}
