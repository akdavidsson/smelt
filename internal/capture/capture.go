package capture

import (
	"fmt"
	"strings"
)

// RegionType classifies what kind of data was found.
type RegionType string

const (
	RegionTable RegionType = "table"
	RegionList  RegionType = "list"
	RegionText  RegionType = "text"
)

// Region represents a discrete chunk of extracted data.
type Region struct {
	Type       RegionType
	RawText    string
	Headers    []string
	Rows       [][]string
	Context    string // surrounding heading or caption
	SourcePage int    // 0 for non-PDF sources
}

// Capturer extracts regions from raw input bytes.
type Capturer interface {
	Capture(input []byte) ([]Region, error)
}

// FormatRegion converts a region to a compact text representation suitable for LLM prompts.
func FormatRegion(r Region) string {
	var sb strings.Builder

	if r.Context != "" {
		sb.WriteString("Context: ")
		sb.WriteString(r.Context)
		sb.WriteString("\n\n")
	}

	if r.Type == RegionTable && len(r.Headers) > 0 {
		sb.WriteString(strings.Join(r.Headers, " | "))
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat("-", 40))
		sb.WriteString("\n")
		for _, row := range r.Rows {
			sb.WriteString(strings.Join(row, " | "))
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString(r.RawText)
	}

	return sb.String()
}

// SelectBestRegion returns the most data-rich region from a list.
// Prefers tables with more rows; if query is non-empty, boosts regions whose
// context heading or headers match the query terms.
func SelectBestRegion(regions []Region, query string) *Region {
	if len(regions) == 0 {
		return nil
	}

	terms := queryTerms(query)
	var best *Region
	bestScore := -1

	for i := range regions {
		r := &regions[i]
		score := scoreRegion(r) + queryBoost(r, terms)
		if score > bestScore {
			bestScore = score
			best = r
		}
	}

	return best
}

// queryTerms lowercases and splits a query string into individual words.
func queryTerms(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// queryBoost returns a score bonus for a region whose context/headers contain query terms.
func queryBoost(r *Region, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	haystack := strings.ToLower(r.Context + " " + strings.Join(r.Headers, " "))
	boost := 0
	for _, t := range terms {
		if strings.Contains(haystack, t) {
			boost += 20
		}
	}
	return boost
}

func scoreRegion(r *Region) int {
	switch r.Type {
	case RegionTable:
		// Prefer tables with more data
		score := len(r.Rows) * 10
		score += len(r.Headers) * 2
		if r.Context != "" {
			score += 5
		}
		return score
	case RegionList:
		return len(r.Rows) * 3
	case RegionText:
		return len(r.RawText) / 100
	}
	return 0
}

// FormatRegionSummary returns a short one-line description of a region.
func FormatRegionSummary(r Region) string {
	switch r.Type {
	case RegionTable:
		ctx := ""
		if r.Context != "" {
			ctx = fmt.Sprintf(" (%s)", r.Context)
		}
		return fmt.Sprintf("table%s: %d cols × %d rows", ctx, len(r.Headers), len(r.Rows))
	case RegionList:
		return fmt.Sprintf("list: %d items", len(r.Rows))
	case RegionText:
		return fmt.Sprintf("text: %d chars", len(r.RawText))
	}
	return "unknown"
}
