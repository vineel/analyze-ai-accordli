package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicDirect is the Phase 1 llm.Client. Talks to api.anthropic.com
// over HTTPS. Vendor "B" in the Reviewer-v2 failover ladder; for now
// it's the only vendor — Foundry (Vendor "A") and the failover wrapper
// land in later phases behind the same Client interface.
//
// API key comes from ANTHRO_API_KEY (deliberately not the standard
// ANTHROPIC_API_KEY — see CLAUDE.md / .env.example).
type AnthropicDirect struct {
	APIKey     string
	Model      string
	MaxTokens  int
	HTTPClient *http.Client
}

func NewAnthropicDirect(apiKey, model string) *AnthropicDirect {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &AnthropicDirect{
		APIKey:    apiKey,
		Model:     model,
		MaxTokens: 8192,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type anthropicContentBlock struct {
	Type         string                       `json:"type"`
	Text         string                       `json:"text"`
	CacheControl *anthropicCacheControl       `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Temperature *float32           `json:"temperature,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type anthropicResponse struct {
	Content []anthropicResponseContent `json:"content"`
	Usage   anthropicUsage             `json:"usage"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *AnthropicDirect) Complete(ctx context.Context, req Request) (*Response, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("ANTHRO_API_KEY not set")
	}

	model := req.Model
	if model == "" {
		model = c.Model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.MaxTokens
	}

	content := make([]anthropicContentBlock, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		blk := anthropicContentBlock{Type: "text", Text: b.Text}
		if b.CacheControl == "ephemeral" {
			blk.CacheControl = &anthropicCacheControl{Type: "ephemeral"}
		}
		content = append(content, blk)
	}

	body := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    req.System,
		Messages: []anthropicMessage{{
			Role:    "user",
			Content: content,
		}},
	}
	if req.Temperature != 0 {
		t := req.Temperature
		body.Temperature = &t
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic call: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(respBytes))
	}

	var out anthropicResponse
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s: %s", out.Error.Type, out.Error.Message)
	}

	var text bytes.Buffer
	for _, c := range out.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}

	return &Response{
		Text:         text.String(),
		InputTokens:  out.Usage.InputTokens,
		OutputTokens: out.Usage.OutputTokens,
		CacheRead:    out.Usage.CacheReadInputTokens,
		CacheWrite:   out.Usage.CacheCreationInputTokens,
		Vendor:       "B",
	}, nil
}
