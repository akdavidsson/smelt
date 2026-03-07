package capture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// HTMLCapturer extracts tabular regions from HTML documents.
type HTMLCapturer struct{}

// Capture parses the HTML and returns all table regions found.
// It first extracts <table> elements, then falls back to embedded JSON
// (e.g. __NEXT_DATA__ from Next.js or application/ld+json).
func (h *HTMLCapturer) Capture(input []byte) ([]Region, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}

	var regions []Region
	doc.Find("table").Each(func(_ int, sel *goquery.Selection) {
		headers, headersFromData := extractTableHeaders(sel)
		rows := extractTableRows(sel, headersFromData)
		if len(rows) == 0 && len(headers) == 0 {
			return
		}
		ctx := findTableContext(sel)

		var rawParts []string
		if len(headers) > 0 {
			rawParts = append(rawParts, strings.Join(headers, "\t"))
		}
		for _, row := range rows {
			rawParts = append(rawParts, strings.Join(row, "\t"))
		}

		regions = append(regions, Region{
			Type:    RegionTable,
			Headers: headers,
			Rows:    rows,
			Context: ctx,
			RawText: strings.Join(rawParts, "\n"),
		})
	})

	// If no HTML tables found, try embedded JSON (Next.js, JSON-LD, etc.)
	if len(regions) == 0 {
		regions = append(regions, extractEmbeddedJSON(doc)...)
	}

	return regions, nil
}

// extractEmbeddedJSON finds <script> tags containing JSON and walks them for
// array-of-objects structures that look like table data.
func extractEmbeddedJSON(doc *goquery.Document) []Region {
	var regions []Region

	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		t, _ := s.Attr("type")
		id, _ := s.Attr("id")

		isJSON := t == "application/json" || t == "application/ld+json" || id == "__NEXT_DATA__"
		if !isJSON {
			return
		}

		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}

		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			return
		}

		regions = append(regions, walkJSONForTables(v, 0)...)
	})

	return regions
}

// walkJSONForTables recursively searches a JSON value for arrays of objects
// that look like tabular data.
func walkJSONForTables(v any, depth int) []Region {
	if depth > 8 {
		return nil
	}

	switch val := v.(type) {
	case []any:
		if r, ok := sliceToRegion(val); ok {
			return []Region{r}
		}
		var regions []Region
		for _, item := range val {
			regions = append(regions, walkJSONForTables(item, depth+1)...)
		}
		return regions

	case map[string]any:
		var regions []Region
		for _, child := range val {
			regions = append(regions, walkJSONForTables(child, depth+1)...)
		}
		return regions
	}
	return nil
}

// sliceToRegion converts a []any to a Region if it looks like tabular data:
// at least 3 elements that are objects with 2+ consistent scalar keys.
func sliceToRegion(arr []any) (Region, bool) {
	if len(arr) < 3 {
		return Region{}, false
	}

	var objects []map[string]any
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			objects = append(objects, m)
		}
	}

	// Need mostly objects
	if len(objects) < 3 || len(objects) < len(arr)/2 {
		return Region{}, false
	}

	// Count key occurrences across all objects
	keyCount := make(map[string]int)
	for _, m := range objects {
		for k := range m {
			keyCount[k]++
		}
	}

	// Keep scalar-valued keys that appear in ≥50% of objects
	threshold := len(objects) / 2
	var headers []string
	for k, count := range keyCount {
		if count < threshold {
			continue
		}
		// Check that value in first object is a scalar
		if v, ok := objects[0][k]; ok {
			switch v.(type) {
			case string, float64, bool, nil:
				headers = append(headers, k)
			}
		}
	}
	sort.Strings(headers)

	if len(headers) < 2 {
		return Region{}, false
	}

	// Build rows
	var rows [][]string
	for _, obj := range objects {
		row := make([]string, len(headers))
		for i, h := range headers {
			if v, ok := obj[h]; ok && v != nil {
				row[i] = fmt.Sprint(v)
			}
		}
		rows = append(rows, row)
	}

	var rawParts []string
	rawParts = append(rawParts, strings.Join(headers, "\t"))
	for _, row := range rows {
		rawParts = append(rawParts, strings.Join(row, "\t"))
	}

	return Region{
		Type:    RegionTable,
		Headers: headers,
		Rows:    rows,
		RawText: strings.Join(rawParts, "\n"),
	}, true
}

// extractTableHeaders tries <th> in <thead> first, then falls back to the first <tr><td>.
// Returns (headers, headersFromData) where headersFromData=true means the first data row was consumed.
func extractTableHeaders(table *goquery.Selection) ([]string, bool) {
	var headers []string

	// Try <thead> <th>
	table.Find("thead th").Each(func(_ int, s *goquery.Selection) {
		headers = append(headers, cleanText(s.Text()))
	})
	if len(headers) > 0 {
		return headers, false
	}

	// Try first <tr> in thead using td
	table.Find("thead tr").First().Find("td").Each(func(_ int, s *goquery.Selection) {
		headers = append(headers, cleanText(s.Text()))
	})
	if len(headers) > 0 {
		return headers, false
	}

	// Fall back to first <tr> <th> anywhere in the table
	table.Find("tr").First().Find("th").Each(func(_ int, s *goquery.Selection) {
		headers = append(headers, cleanText(s.Text()))
	})
	if len(headers) > 0 {
		return headers, false // th rows are already skipped in extractTableRows
	}

	// Fall back to first <tr> <td> as headers (consumes data row)
	table.Find("tr").First().Find("td").Each(func(_ int, s *goquery.Selection) {
		headers = append(headers, cleanText(s.Text()))
	})
	return headers, len(headers) > 0
}

// extractTableRows extracts data rows from <tbody>, skipping the header row if skipFirst is true.
func extractTableRows(table *goquery.Selection, skipFirst bool) [][]string {
	var rows [][]string
	skipped := false

	table.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		// Skip rows that are purely header rows
		if tr.Find("th").Length() > 0 {
			return
		}

		if skipFirst && !skipped {
			skipped = true
			return
		}

		var cells []string
		tr.Find("td").Each(func(_ int, td *goquery.Selection) {
			cells = append(cells, cleanText(td.Text()))
		})
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	})

	return rows
}

// findTableContext walks the DOM for a nearby heading or caption.
func findTableContext(table *goquery.Selection) string {
	// Check for <caption> first
	caption := cleanText(table.Find("caption").First().Text())
	if caption != "" {
		return caption
	}

	// Check for <figcaption> in parent figure
	figcap := cleanText(table.Closest("figure").Find("figcaption").First().Text())
	if figcap != "" {
		return figcap
	}

	// Walk previous siblings and parents for headings
	headings := []string{"h1", "h2", "h3", "h4", "h5", "h6"}
	sel := table

	for i := 0; i < 5; i++ {
		prev := sel.Prev()
		if prev.Length() == 0 {
			// Go up a level
			sel = sel.Parent()
			if sel.Length() == 0 {
				break
			}
			continue
		}

		for _, h := range headings {
			if goquery.NodeName(prev) == h {
				return cleanText(prev.Text())
			}
		}
		// Check descendants for headings
		for _, h := range headings {
			text := cleanText(prev.Find(h).Last().Text())
			if text != "" {
				return text
			}
		}
		sel = prev
	}

	return ""
}

// cleanText trims and collapses internal whitespace.
func cleanText(s string) string {
	s = strings.TrimSpace(s)
	// Collapse all internal whitespace sequences to a single space
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
