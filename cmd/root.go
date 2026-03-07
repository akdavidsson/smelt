package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akdavidsson/smelt/internal/capture"
	"github.com/akdavidsson/smelt/internal/config"
	"github.com/akdavidsson/smelt/internal/extract"
	"github.com/akdavidsson/smelt/internal/output"
	"github.com/akdavidsson/smelt/internal/schema"
)

var (
	flagFormat  string
	flagOutput  string
	flagQuery   string
	flagSchema  bool
	flagOCR     bool
	flagModel   string
	flagRaw     bool
	flagVerbose bool
	flagTable   int
	flagAll     bool
)

var rootCmd = &cobra.Command{
	Use:   "smelt [input]",
	Short: "Extract structured data from PDFs and HTML using AI-inferred schemas",
	Long: `smelt takes a PDF or HTML file, extracts tabular data, infers a schema
using the Anthropic API, and outputs JSON or CSV.`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          run,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Flags().StringVarP(&flagFormat, "format", "f", "json", "output format: json, csv, parquet")
	rootCmd.Flags().StringVarP(&flagOutput, "output", "o", "", "write output to file instead of stdout")
	rootCmd.Flags().StringVarP(&flagQuery, "query", "q", "", "natural language hint for schema inference")
	rootCmd.Flags().BoolVar(&flagSchema, "schema", false, "print inferred schema as JSON and exit")
	rootCmd.Flags().BoolVar(&flagOCR, "ocr", false, "enable OCR (not yet implemented)")
	rootCmd.Flags().StringVar(&flagModel, "model", "", "Anthropic model to use (overrides config)")
	rootCmd.Flags().BoolVar(&flagRaw, "raw", false, "print extracted regions to stderr and exit")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "enable verbose logging")
	rootCmd.Flags().IntVar(&flagTable, "table", 0, "select Nth table (1-based); 0 = auto-select")
	rootCmd.Flags().BoolVar(&flagAll, "all", false, "extract all tables and output as JSON array")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func run(cmd *cobra.Command, args []string) error {
	// 1. Load config (validate API key only when needed)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Verbose = flagVerbose
	if flagModel != "" {
		cfg.Model = flagModel
	}

	// 2. Read input
	var inputData []byte
	var inputName string

	if len(args) == 0 {
		// Read from stdin
		inputName = "stdin"
		inputData, err = os.ReadFile("/dev/stdin")
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	} else {
		inputName = args[0]
		if strings.HasPrefix(inputName, "http://") || strings.HasPrefix(inputName, "https://") {
			inputData, err = fetchURL(inputName)
			if err != nil {
				return fmt.Errorf("fetching %s: %w", inputName, err)
			}
		} else {
			inputData, err = os.ReadFile(inputName)
			if err != nil {
				return fmt.Errorf("reading %s: %w", inputName, err)
			}
		}
	}

	// 3. Detect input type and create capturer
	inputType := detectInputType(inputName, inputData)
	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "detected input type: %s\n", inputType)
	}

	var capturer capture.Capturer
	switch inputType {
	case "pdf":
		capturer = &capture.PDFCapturer{}
	case "html":
		capturer = &capture.HTMLCapturer{}
	default:
		return fmt.Errorf("unsupported input type for %q; use a .pdf or .html file", inputName)
	}

	// 4. Capture regions
	regions, err := capturer.Capture(inputData)
	if err != nil {
		return fmt.Errorf("extracting content: %w", err)
	}

	if len(regions) == 0 {
		return fmt.Errorf("no extractable regions found in input")
	}

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "found %d region(s)\n", len(regions))
		for i, r := range regions {
			fmt.Fprintf(os.Stderr, "  [%d] %s\n", i, capture.FormatRegionSummary(r))
		}
	}

	// 5. If --raw, print regions and exit (no API key needed)
	if flagRaw {
		for i, r := range regions {
			fmt.Fprintf(os.Stderr, "\n--- Region %d (%s) ---\n", i+1, capture.FormatRegionSummary(r))
			fmt.Fprintln(os.Stderr, capture.FormatRegion(r))
		}
		return nil
	}

	// Validate API key now that we need it
	if err := cfg.Validate(); err != nil {
		return err
	}

	// 6. --all: extract every table
	if flagAll {
		return runAll(cmd, regions, cfg)
	}

	// 7. Select region: --table N (1-based) or auto
	var region *capture.Region
	if flagTable > 0 {
		if flagTable > len(regions) {
			return fmt.Errorf("--table %d out of range: only %d region(s) found", flagTable, len(regions))
		}
		region = &regions[flagTable-1]
	} else {
		region = capture.SelectBestRegion(regions, flagQuery)
		if region == nil {
			return fmt.Errorf("could not select a region")
		}
	}

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "selected region: %s\n", capture.FormatRegionSummary(*region))
	}

	// 8. Infer schema
	inferredSchema, err := inferRegion(cmd.Context(), *region, cfg)
	if err != nil {
		return err
	}

	// 9. If --schema, print schema and exit
	if flagSchema {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(inferredSchema)
	}

	// 10. Extract structured data
	result, err := extract.Extract(*region, *inferredSchema)
	if err != nil {
		return fmt.Errorf("extraction: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	// 11. Write output
	return writeOutput(result, flagFormat, flagOutput)
}

// fetchURL downloads a URL and returns its body.
func fetchURL(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; smelt/1.0; +https://github.com/akdavidsson/smelt)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// detectInputType returns "pdf", "html", or "unknown".
func detectInputType(name string, data []byte) string {
	// For URLs, strip query/fragment before checking extension
	cleanName := name
	if i := strings.IndexAny(cleanName, "?#"); i != -1 {
		cleanName = cleanName[:i]
	}
	ext := strings.ToLower(filepath.Ext(cleanName))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".html", ".htm":
		return "html"
	}

	// Check magic bytes
	if len(data) >= 4 && string(data[:4]) == "%PDF" {
		return "pdf"
	}

	// Content sniff for HTML
	snippet := strings.ToLower(strings.TrimSpace(string(data[:min(512, len(data))])))
	if strings.HasPrefix(snippet, "<!doctype html") ||
		strings.HasPrefix(snippet, "<html") ||
		strings.Contains(snippet, "<table") {
		return "html"
	}

	return "unknown"
}

// inferRegion calls the Anthropic API to infer a schema for one region.
func inferRegion(ctx context.Context, region capture.Region, cfg *config.Config) (*schema.Schema, error) {
	if cfg.Verbose {
		fmt.Fprintln(os.Stderr, "calling Anthropic API for schema inference...")
	}
	s, err := schema.Infer(ctx, schema.InferenceRequest{
		RegionText: capture.FormatRegion(region),
		Query:      flagQuery,
		Model:      cfg.Model,
		APIKey:     cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("schema inference: %w", err)
	}
	return s, nil
}

// runAll extracts every region and writes a JSON array of {name, context, records}.
func runAll(cmd *cobra.Command, regions []capture.Region, cfg *config.Config) error {
	if flagFormat != "json" {
		return fmt.Errorf("--all only supports --format json; CSV with multiple tables is not supported")
	}

	var tables []output.TableResult
	for i, region := range regions {
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "processing region %d/%d: %s\n", i+1, len(regions), capture.FormatRegionSummary(region))
		}
		s, err := inferRegion(cmd.Context(), region, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: region %d schema inference failed: %v — skipping\n", i+1, err)
			continue
		}
		result, err := extract.Extract(region, *s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: region %d extraction failed: %v — skipping\n", i+1, err)
			continue
		}
		for _, w := range result.Warnings {
			fmt.Fprintf(os.Stderr, "warning: region %d: %s\n", i+1, w)
		}
		tables = append(tables, output.TableResult{
			Name:    s.Name,
			Context: region.Context,
			Records: result.Records,
		})
	}

	w := os.Stdout
	if flagOutput != "" {
		f, err := os.Create(flagOutput)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		w = f
	}
	return output.WriteAllJSON(w, tables)
}

// writeOutput selects the writer and writes to stdout or a file.
func writeOutput(result *extract.Result, format, outputPath string) error {
	w := os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	switch strings.ToLower(format) {
	case "json":
		return output.WriteJSON(w, result)
	case "csv":
		return output.WriteCSV(w, result)
	case "parquet":
		return output.WriteParquet(w, result)
	default:
		return fmt.Errorf("unknown format %q; use json, csv, or parquet", format)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
