package post

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
)

// ── chat driver interface (self-contained, avoids entity/models import) ──

// ChatDriver is the subset of modelModule.ModelDriver needed to call a
// vision-capable chat API.  Defined here to keep model_image_describer.go
// self-contained and avoid import chains that require CGO.
type ChatDriver interface {
	ChatWithMessages(modelName string, messages []ChatMessage, apiConfig *ChatAPIConfig, chatConfig *ChatConfig) (*ChatResponse, error)
}

// ChatMessage mirrors modelModule.Message.
type ChatMessage struct {
	Role       string                   `json:"role"`
	Content    interface{}              `json:"content"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
	ToolCalls  []map[string]interface{} `json:"tool_calls,omitempty"`
}

// ChatAPIConfig mirrors modelModule.APIConfig.
type ChatAPIConfig struct {
	ApiKey  *string
	Region  *string
	BaseURL *string
}

// ChatConfig mirrors modelModule.ChatConfig (may be nil).
type ChatConfig struct{}

// ChatResponse mirrors modelModule.ChatResponse.
type ChatResponse struct {
	Answer        *string                  `json:"answer"`
	ReasonContent *string                  `json:"reason_content"`
	ToolCalls     []map[string]interface{} `json:"tool_calls,omitempty"`
}

// ── ModelImageDescriber ────────────────────────────────────────────────

// ModelImageDescriber implements ImageDescriber via any ChatDriver.
type ModelImageDescriber struct {
	driver    ChatDriver
	modelName string
	apiConfig *ChatAPIConfig
	maxTokens int
}

// NewModelImageDescriber creates a ModelImageDescriber that calls the given
// driver to describe images. maxTokens sets the response length limit (passed
// as ChatConfig.MaxTokens); 0 means use provider default.
func NewModelImageDescriber(d ChatDriver, name string, cfg *ChatAPIConfig, maxTokens int) *ModelImageDescriber {
	return &ModelImageDescriber{driver: d, modelName: name, apiConfig: cfg, maxTokens: maxTokens}
}

// DescribeImage sends the image as a base64 data URL in an OpenAI-compatible
// vision API request.  Returns the model's text response.
func (d *ModelImageDescriber) DescribeImage(ctx context.Context, img image.Image, prompt string) (string, error) {
	dataURL, err := encodeImageToBase64DataURL(img)
	if err != nil {
		return "", fmt.Errorf("image encode: %w", err)
	}

	msgs := []ChatMessage{{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": prompt},
			map[string]interface{}{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
		},
	}}

	var chatCfg *ChatConfig
	if d.maxTokens > 0 {
		chatCfg = &ChatConfig{}
	}
	resp, err := d.driver.ChatWithMessages(d.modelName, msgs, d.apiConfig, chatCfg)
	if err != nil {
		return "", fmt.Errorf("image describe: %w", err)
	}
	if resp.Answer == nil || *resp.Answer == "" {
		return "", errors.New("image describe: empty response")
	}
	return *resp.Answer, nil
}

// encodeImageToBase64DataURL encodes an image as a PNG data URL.
func encodeImageToBase64DataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
