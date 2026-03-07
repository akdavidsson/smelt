package capture

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// PDFCapturer extracts text regions from PDF documents.
type PDFCapturer struct{}

// Capture extracts text from a PDF and returns detected regions.
// It tries pdftotext first (handles font encoding correctly), then falls
// back to parsing raw pdfcpu content streams.
func (p *PDFCapturer) Capture(input []byte) ([]Region, error) {
	tmpDir, err := os.MkdirTemp("", "smelt-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, input, 0600); err != nil {
		return nil, fmt.Errorf("writing temp pdf: %w", err)
	}

	// Prefer pdftotext: it resolves font encoding (ToUnicode CMap) correctly.
	if path, err := exec.LookPath("pdftotext"); err == nil {
		regions, err := extractViaPdftotext(path, pdfPath)
		if err == nil && len(regions) > 0 {
			return regions, nil
		}
	}

	// Fallback: parse raw pdfcpu content streams.
	return extractViaPdfcpu(pdfPath, tmpDir)
}

// extractViaPdftotext runs pdftotext -layout and parses its output.
func extractViaPdftotext(pdftotextBin, pdfPath string) ([]Region, error) {
	out, err := exec.Command(pdftotextBin, "-layout", pdfPath, "-").Output()
	if err != nil {
		return nil, fmt.Errorf("pdftotext: %w", err)
	}

	// pdftotext separates pages with form-feed (\f)
	pages := strings.Split(string(out), "\f")
	var allRegions []Region
	for pageNum, pageText := range pages {
		allRegions = append(allRegions, detectRegions(pageText, pageNum+1)...)
	}
	return allRegions, nil
}

// extractViaPdfcpu extracts and parses raw PDF content streams.
func extractViaPdfcpu(pdfPath, tmpDir string) ([]Region, error) {
	contentDir := filepath.Join(tmpDir, "content")
	if err := os.MkdirAll(contentDir, 0700); err != nil {
		return nil, fmt.Errorf("creating content dir: %w", err)
	}

	if err := api.ExtractContentFile(pdfPath, contentDir, nil, nil); err != nil {
		return nil, fmt.Errorf("extracting pdf content: %w", err)
	}

	entries, err := os.ReadDir(contentDir)
	if err != nil {
		return nil, fmt.Errorf("reading content dir: %w", err)
	}

	var allRegions []Region
	for pageNum, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(contentDir, entry.Name()))
		if err != nil {
			continue
		}
		pageText := parseContentStream(string(data))
		allRegions = append(allRegions, detectRegions(pageText, pageNum+1)...)
	}
	return allRegions, nil
}

var (
	reTj  = regexp.MustCompile(`\(([^)]*(?:\\.[^)]*)*)\)\s*Tj`)
	reTJ  = regexp.MustCompile(`\[([^\]]+)\]\s*TJ`)
	reStr = regexp.MustCompile(`\(([^)]*(?:\\.[^)]*)*)\)`)
)

// parseContentStream extracts human-readable text from a PDF content stream.
func parseContentStream(stream string) string {
	var parts []string

	for _, m := range reTj.FindAllStringSubmatch(stream, -1) {
		parts = append(parts, unescapePDF(m[1]))
	}

	for _, m := range reTJ.FindAllStringSubmatch(stream, -1) {
		inner := m[1]
		for _, sm := range reStr.FindAllStringSubmatch(inner, -1) {
			parts = append(parts, unescapePDF(sm[1]))
		}
	}

	return strings.Join(parts, " ")
}

// unescapePDF handles basic PDF string escape sequences.
func unescapePDF(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	return s
}

// detectRegions applies heuristics to find table-like structures in extracted text.
func detectRegions(pageText string, pageNum int) []Region {
	if strings.TrimSpace(pageText) == "" {
		return nil
	}

	lines := strings.Split(pageText, "\n")

	var nonEmpty []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}

	if len(nonEmpty) == 0 {
		return nil
	}

	// Look for lines with 2+ space separators (table-like alignment)
	type alignedLine struct {
		raw    string
		tokens []string
	}

	var aligned []alignedLine
	reMultiSpace := regexp.MustCompile(`\s{2,}`)

	for _, line := range nonEmpty {
		tokens := reMultiSpace.Split(strings.TrimSpace(line), -1)
		if len(tokens) >= 2 {
			aligned = append(aligned, alignedLine{raw: line, tokens: tokens})
		}
	}

	var regions []Region

	if len(aligned) >= 3 {
		headers := aligned[0].tokens
		var rows [][]string
		for _, al := range aligned[1:] {
			rows = append(rows, al.tokens)
		}

		ctx := extractPDFContext(nonEmpty, 0)

		var rawLines []string
		for _, al := range aligned {
			rawLines = append(rawLines, al.raw)
		}

		regions = append(regions, Region{
			Type:       RegionTable,
			Headers:    headers,
			Rows:       rows,
			Context:    ctx,
			RawText:    strings.Join(rawLines, "\n"),
			SourcePage: pageNum,
		})
	} else {
		regions = append(regions, Region{
			Type:       RegionText,
			RawText:    strings.Join(nonEmpty, "\n"),
			SourcePage: pageNum,
		})
	}

	return regions
}

// extractPDFContext looks backward from startIdx for a context line.
func extractPDFContext(lines []string, startIdx int) string {
	limit := startIdx - 10
	if limit < 0 {
		limit = 0
	}

	for i := startIdx - 1; i >= limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		isAllCaps := true
		hasDigit := false
		for _, r := range line {
			if unicode.IsLetter(r) && !unicode.IsUpper(r) {
				isAllCaps = false
			}
			if unicode.IsDigit(r) {
				hasDigit = true
			}
		}
		if (isAllCaps && len(line) > 3) || (!hasDigit && len(line) < 60) {
			return line
		}
	}
	return ""
}
