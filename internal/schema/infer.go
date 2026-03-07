package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicMessagesURL = "https://api.anthropic.com/v1/messages"
	anthropicVersion     = "2023-06-01"
	inferMaxTokens       = 1024
	inferTimeout         = 30 * time.Second
)

const systemPrompt = `You are a data schema inference engine. Your only output is a valid JSON object.

Respond ONLY with a JSON object matching this exact structure:
{"name":"...","description":"...","columns":[{"name":"...","type":"string|int|float|bool|date|datetime","nullable":true|false,"description":"..."}]}

Rules:
- column names must be snake_case
- types must be exactly one of: string, int, float, bool, date, datetime
- no markdown fences, no explanation, no text outside the JSON object
- infer nullable=true only if the data clearly has missing values
- prefer flat schemas; only nest columns if the data is genuinely hierarchical`

// InferenceRequest holds the inputs for a schema inference call.
type InferenceRequest struct {
	RegionText string
	Query      string
	Model      string
	APIKey     string
}

// anthropicMessage is used for the API request body.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Infer calls the Anthropic API to infer a schema from extracted region text.
func Infer(ctx context.Context, req InferenceRequest) (*Schema, error) {
	userMsg := buildUserMessage(req.RegionText, req.Query)

	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: inferMaxTokens,
		System:    systemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: userMsg}},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: inferTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling Anthropic API: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic API returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("Anthropic API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Anthropic API")
	}

	rawText := apiResp.Content[0].Text
	return parseSchemaFromResponse(rawText)
}

// buildUserMessage constructs the LLM prompt.
func buildUserMessage(regionText, query string) string {
	var sb strings.Builder
	if query != "" {
		sb.WriteString("User query: ")
		sb.WriteString(query)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Data sample:\n```\n")
	sb.WriteString(regionText)
	sb.WriteString("\n```\n\nInfer the schema for this data.")
	return sb.String()
}

// extractJSON strips markdown fences and finds the JSON object boundaries.
func extractJSON(text string) string {
	// Strip markdown code fences
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	// Find first { and last }
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end < start {
		return text
	}
	return text[start : end+1]
}

// parseSchemaFromResponse unmarshals and validates the LLM's JSON output.
func parseSchemaFromResponse(text string) (*Schema, error) {
	jsonStr := extractJSON(text)

	var s Schema
	if err := json.Unmarshal([]byte(jsonStr), &s); err != nil {
		return nil, fmt.Errorf("parsing schema JSON: %w\nraw: %s", err, jsonStr)
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	return &s, nil
}
