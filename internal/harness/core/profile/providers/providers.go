// Package providers contains built-in provider adapters for the profile system.
// Each adapter registers itself in the global provider registry.
//
// Supported providers:
//   - Anthropic ("anthropic"): Claude models via Anthropic Messages API
//   - OpenAI ("openai"): GPT models via OpenAI Chat Completions API
//
// Both use net/http directly (no external SDK dependency).
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/profile"
	"ragflow/internal/harness/core/schema"
)

// ========================================================================
// Shared HTTP helpers
// ========================================================================

const defaultTimeout = 60 * time.Second

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultTimeout}
}

func doRequest(ctx context.Context, client httpClient, req *http.Request, dst any) error {
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("parse response: %w (body: %s)", err, string(body))
	}
	return nil
}

// ========================================================================
// Anthropic provider
// ========================================================================

const defaultAnthropicBaseURL = "https://api.anthropic.com/v1"

type anthropicProvider struct{}

// AnthropicConfig carries provider-level settings for the Anthropic adapter.
type AnthropicConfig struct {
	APIKey      string
	BaseURL     string
	Temperature float64
	MaxTokens   int
}

// RegisterAnthropic registers the Anthropic provider with the global registry.
func RegisterAnthropic(cfg AnthropicConfig) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultAnthropicBaseURL
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}

	profile.RegisterProvider(&profile.ProviderProfile{
		Name:         "anthropic",
		DefaultModel: "claude-sonnet-4-6",
		DefaultOpts: map[string]any{
			"api_key":     cfg.APIKey,
			"api_base":    cfg.BaseURL,
			"temperature": cfg.Temperature,
			"max_tokens":  float64(cfg.MaxTokens),
		},
		InitModel: func(ctx context.Context, modelName string, opts map[string]any) (core.Model[*schema.Message], error) {
			return newAnthropicModel(modelName, opts), nil
		},
	})
}

func newAnthropicModel(model string, opts map[string]any) *anthropicModel {
	return &anthropicModel{
		model:       model,
		apiKey:      getStr(opts, "api_key"),
		baseURL:     getStr(opts, "api_base", defaultAnthropicBaseURL),
		temperature: getFloat(opts, "temperature"),
		maxTokens:   int(getFloat(opts, "max_tokens", 4096)),
		client:      defaultHTTPClient(),
	}
}

type anthropicModel struct {
	model       string
	apiKey      string
	baseURL     string
	temperature float64
	maxTokens   int
	client      httpClient
	tools       []*schema.ToolInfo
}

// ---- Anthropic API types ----

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Name  string `json:"name,omitempty"` // for tool_use
	Input any    `json:"input,omitempty"`
	ID    string `json:"id,omitempty"` // for tool_use/result
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
	Tools       []anthropicToolDef `json:"tools,omitempty"`
}

type anthropicToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type anthropicResponse struct {
	Content []contentBlock `json:"content"`
	StopReason string     `json:"stop_reason"`
}

func (m *anthropicModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	req := m.buildRequest(msgs)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, m.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", m.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	var resp anthropicResponse
	if err := doRequest(ctx, m.client, httpReq, &resp); err != nil {
		return nil, err
	}

	return m.convertResponse(&resp), nil
}

func (m *anthropicModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	// Non-streaming fallback for simplicity.
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *anthropicModel) BindTools(tools []*schema.ToolInfo) error {
	m.tools = tools
	return nil
}

func (m *anthropicModel) buildRequest(msgs []*schema.Message) *anthropicRequest {
	req := &anthropicRequest{
		Model:       m.model,
		MaxTokens:   m.maxTokens,
		Temperature: m.temperature,
	}

	var systemText string
	for _, msg := range msgs {
		if msg.Role == schema.RoleSystem {
			systemText += msg.Content + "\n"
			continue
		}
		var content any = msg.Content
		if len(msg.ToolCalls) > 0 {
			var blocks []contentBlock
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, contentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: json.RawMessage(tc.Function.Arguments),
				})
			}
			content = blocks
		}
		role := msg.Role
		if role == schema.RoleTool {
			role = "user"
			content = []contentBlock{{
				Type: "tool_result",
				ID:   msg.Name,
				Text: msg.Content,
			}}
		}
		req.Messages = append(req.Messages, anthropicMessage{
			Role:    string(role),
			Content: content,
		})
	}

	if systemText != "" {
		req.System = strings.TrimSuffix(systemText, "\n")
	}

	// Populate tools.
	for _, t := range m.tools {
		req.Tools = append(req.Tools, anthropicToolDef{
			Name:        t.Name,
			Description: t.Description,
		})
	}

	return req
}

func (m *anthropicModel) convertResponse(resp *anthropicResponse) *schema.Message {
	var content string
	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}
	msg := &schema.Message{
		Role:    schema.RoleAssistant,
		Content: content,
	}
	// Tool calls.
	for _, block := range resp.Content {
		if block.Type == "tool_use" && block.ID != "" {
			args, _ := json.Marshal(block.Input)
			msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
				ID: block.ID,
				Function: schema.ToolCallFunction{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}
	return msg
}

// ========================================================================
// OpenAI provider
// ========================================================================

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type openAIProvider struct{}

// OpenAIConfig carries provider-level settings for the OpenAI adapter.
type OpenAIConfig struct {
	APIKey      string
	BaseURL     string
	Temperature float64
	MaxTokens   int
}

// RegisterOpenAI registers the OpenAI provider with the global registry.
func RegisterOpenAI(cfg OpenAIConfig) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultOpenAIBaseURL
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}

	profile.RegisterProvider(&profile.ProviderProfile{
		Name:         "openai",
		DefaultModel: "gpt-4o",
		DefaultOpts: map[string]any{
			"api_key":     cfg.APIKey,
			"api_base":    cfg.BaseURL,
			"temperature": cfg.Temperature,
			"max_tokens":  float64(cfg.MaxTokens),
		},
		InitModel: func(ctx context.Context, modelName string, opts map[string]any) (core.Model[*schema.Message], error) {
			return newOpenAIModel(modelName, opts), nil
		},
	})
}

func newOpenAIModel(model string, opts map[string]any) *openAIModel {
	return &openAIModel{
		model:       model,
		apiKey:      getStr(opts, "api_key"),
		baseURL:     getStr(opts, "api_base", defaultOpenAIBaseURL),
		temperature: getFloat(opts, "temperature"),
		maxTokens:   int(getFloat(opts, "max_tokens", 4096)),
		client:      defaultHTTPClient(),
	}
}

type openAIModel struct {
	model       string
	apiKey      string
	baseURL     string
	temperature float64
	maxTokens   int
	client      httpClient
}

// ---- OpenAI API types ----

type openAIMessage struct {
	Role        string           `json:"role"`
	Content     any              `json:"content,omitempty"` // string or []contentPart
	ToolCalls   []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string           `json:"tool_call_id,omitempty"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type openAIToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function openAIFunction   `json:"function"`
}

type openAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Tools       []openAIToolDef  `json:"tools,omitempty"`
}

type openAIToolDef struct {
	Type     string         `json:"type"`
	Function openAIFuncDef  `json:"function"`
}

type openAIFuncDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

func (m *openAIModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	req := m.buildRequest(msgs)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, m.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)

	var resp openAIResponse
	if err := doRequest(ctx, m.client, httpReq, &resp); err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	return m.convertMessage(&resp.Choices[0].Message), nil
}

func (m *openAIModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *openAIModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func (m *openAIModel) buildRequest(msgs []*schema.Message) *openAIRequest {
	req := &openAIRequest{
		Model:       m.model,
		Temperature: m.temperature,
		MaxTokens:   m.maxTokens,
	}
	for _, msg := range msgs {
		om := openAIMessage{Role: string(msg.Role)}
		switch msg.Role {
		case schema.RoleSystem:
			om.Role = "system"
			om.Content = msg.Content
		case schema.RoleAssistant:
			om.Role = "assistant"
			om.Content = msg.Content
			for _, tc := range msg.ToolCalls {
				om.ToolCalls = append(om.ToolCalls, openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		case schema.RoleTool:
			om.Role = "tool"
			om.Content = msg.Content
			om.ToolCallID = msg.Name
		default:
			om.Role = "user"
			om.Content = msg.Content
		}
		req.Messages = append(req.Messages, om)
	}
	return req
}

func (m *openAIModel) convertMessage(om *openAIMessage) *schema.Message {
	msg := &schema.Message{
		Role:    schema.RoleAssistant,
		Content: getStringContent(om.Content),
	}
	for _, tc := range om.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
			ID: tc.ID,
			Function: schema.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return msg
}

// ========================================================================
// Utility functions
// ========================================================================

func getStr(m map[string]any, key string, defaults ...string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return ""
}

func getFloat(m map[string]any, key string, defaults ...float64) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return 0
}

func getStringContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// RegisterAll is a convenience function that registers both Anthropic and OpenAI
// providers with their respective configs. Call it once in your main function.
func RegisterAll(anthropicCfg AnthropicConfig, openaiCfg OpenAIConfig) {
	RegisterAnthropic(anthropicCfg)
	RegisterOpenAI(openaiCfg)
}
