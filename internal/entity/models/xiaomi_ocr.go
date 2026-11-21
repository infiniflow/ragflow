// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"strings"
	"time"

	"ragflow/internal/common"

	"golang.org/x/image/draw"
)

const xiaomiOCRMaxEdge = 2048

// The whole-page prompt from Xiaomi's reference inference pipeline.
const xiaomiPagePrompt = "Extract all information from the main body of the document image and represent it in markdown format, ignoring headers and footers. Tables should be expressed in OTSL format, formulas in the document should be represented using LATEX format, and the parsing should be organized according to the reading order."

type xiaomiOCRModel struct {
	*VllmModel
}

func (m *xiaomiOCRModel) Name() string { return "xiaomiocr" }

func (m *xiaomiOCRModel) NewInstance(urls map[string]string) ModelDriver {
	return &xiaomiOCRModel{NewVllmModel(urls, m.baseModel.URLSuffix)}
}

// The shared OCR provisioner stores its configuration in the instance's API key column.
func xiaomiOCRConfig(config *APIConfig) *APIConfig {
	if url := ProviderJSONConfigValueFromAPIConfig(config, common.EnvXiaomiOCRServerURL); url != "" {
		return &APIConfig{BaseURL: &url}
	}
	return config
}

func (m *xiaomiOCRModel) ListModels(ctx context.Context, config *APIConfig) ([]ListModelResponse, error) {
	models, err := m.VllmModel.ListModels(ctx, xiaomiOCRConfig(config))
	for i := range models {
		models[i].ModelTypes = []string{"ocr"}
	}
	return models, err
}

func (m *xiaomiOCRModel) CheckConnection(ctx context.Context, config *APIConfig) error {
	_, err := m.ListModels(ctx, config)
	return err
}

// Verify the serving endpoint without sending the generic one-pixel OCR probe.
func (m *xiaomiOCRModel) VerifyOCRConnection(ctx context.Context, config *APIConfig) error {
	return m.CheckConnection(ctx, config)
}

func (m *xiaomiOCRModel) OCRFile(ctx context.Context, model *string, content []byte, _ *string, config *APIConfig, options *OCRConfig, usage *common.ModelUsage) (*OCRFileResponse, error) {
	if model == nil || strings.TrimSpace(*model) == "" {
		return nil, fmt.Errorf("XiaomiOCR requires a served model name")
	}
	config = xiaomiOCRConfig(config)
	endpoint, err := m.baseModel.EndpointURL(config, m.baseModel.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	var total TokenUsage
	recognize := func(page image.Image) (string, error) {
		imageURL, err := xiaomiOCRImageURL(page)
		if err != nil {
			return "", err
		}
		return m.recognizePage(ctx, endpoint, *model, imageURL, config, &total)
	}
	var result *OCRFileResponse
	if ocrMIMEType(content) == "application/pdf" {
		result, err = xiaomiOCRPDF(ctx, content, options, recognize)
	} else {
		if len(content) == 0 {
			return nil, fmt.Errorf("XiaomiOCR requires image or PDF bytes")
		}
		size, _, decodeErr := image.DecodeConfig(bytes.NewReader(content))
		if decodeErr != nil {
			return nil, fmt.Errorf("XiaomiOCR image: %w", decodeErr)
		}
		// Decode only after bounding the source raster, even when the encoded upload is small.
		const maxSourcePixels = 40_000_000
		if size.Width <= 0 || size.Height <= 0 || size.Width > maxSourcePixels/size.Height {
			return nil, fmt.Errorf("XiaomiOCR image dimensions exceed the raster limit: %dx%d", size.Width, size.Height)
		}
		page, _, decodeErr := image.Decode(bytes.NewReader(content))
		if decodeErr != nil {
			return nil, fmt.Errorf("XiaomiOCR image: %w", decodeErr)
		}
		var text string
		text, err = recognize(page)
		result = &OCRFileResponse{Text: &text}
	}
	if err != nil {
		return nil, err
	}
	if err := collectModelUsage(usage, &total); err != nil {
		common.Error("Failed to collect XiaomiOCR usage", err)
	}
	return result, nil
}

func xiaomiOCRImageURL(src image.Image) (string, error) {
	size := src.Bounds().Size()
	if edge := max(size.X, size.Y); edge > xiaomiOCRMaxEdge {
		dst := image.NewRGBA(image.Rect(0, 0, max(1, size.X*xiaomiOCRMaxEdge/edge), max(1, size.Y*xiaomiOCRMaxEdge/edge)))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		src = dst
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, src); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes()), nil
}

func (m *xiaomiOCRModel) recognizePage(ctx context.Context, endpoint, model, imageURL string, config *APIConfig, total *TokenUsage) (string, error) {
	payload := map[string]any{
		"model": model, "messages": []Message{{Role: "user", Content: []map[string]any{
			{"type": "image_url", "image_url": map[string]string{"url": imageURL}},
			{"type": "text", "text": xiaomiPagePrompt},
		}}},
		"temperature": 0, "top_p": 0.8, "max_tokens": 4096,
		"repetition_penalty": 1.0, "presence_penalty": 0.0,
		"chat_template_kwargs": map[string]bool{"enable_thinking": false},
	}
	body, err := m.baseModel.doRequest(ctx, endpoint, config, payload, 5*time.Minute)
	if err != nil {
		return "", err
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Usage TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("XiaomiOCR response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("XiaomiOCR returned no choices")
	}
	choice := response.Choices[0]
	if choice.Finish != "stop" && choice.Finish != "" {
		return "", fmt.Errorf("XiaomiOCR incomplete page (finish_reason %q)", choice.Finish)
	}
	if choice.Message.Content == nil {
		return "", fmt.Errorf("XiaomiOCR returned no page content")
	}
	total.PromptTokens += response.Usage.PromptTokens
	total.CompletionTokens += response.Usage.CompletionTokens
	total.TotalTokens += response.Usage.TotalTokens
	return xiaomiOCRMarkdown(*choice.Message.Content), nil
}
