package post

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"

	modelModule "ragflow/internal/entity/models"
)

// ── ModelImageDescriber ────────────────────────────────────────────────

// ModelImageDescriber implements ImageDescriber via any modelModule.ModelDriver.
type ModelImageDescriber struct {
	driver    modelModule.ModelDriver
	modelName string
	apiConfig *modelModule.APIConfig
	maxTokens int
}

// NewModelImageDescriber creates a ModelImageDescriber that calls the given
// driver to describe images. maxTokens sets the response length limit (passed
// as ChatConfig.MaxTokens); 0 means use provider default.
func NewModelImageDescriber(d modelModule.ModelDriver, name string, cfg *modelModule.APIConfig, maxTokens int) *ModelImageDescriber {
	return &ModelImageDescriber{driver: d, modelName: name, apiConfig: cfg, maxTokens: maxTokens}
}

// DescribeImage sends the image as a base64 data URL in an OpenAI-compatible
// vision API request.  Returns the model's text response.
func (d *ModelImageDescriber) DescribeImage(ctx context.Context, img image.Image, prompt string) (string, error) {
	dataURL, err := encodeImageToBase64DataURL(img)
	if err != nil {
		return "", fmt.Errorf("image encode: %w", err)
	}

	msgs := []modelModule.Message{{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": prompt},
			map[string]interface{}{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
		},
	}}

	var chatCfg *modelModule.ChatConfig
	if d.maxTokens > 0 {
		mt := d.maxTokens
		chatCfg = &modelModule.ChatConfig{MaxTokens: &mt}
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
