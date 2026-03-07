package capture

import (
	"bytes"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// HTMLCapturer extracts tabular regions from HTML documents.
type HTMLCapturer struct{}

// Capture parses the HTML and returns all table regions found.
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

	return regions, nil
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
