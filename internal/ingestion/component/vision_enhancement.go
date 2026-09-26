//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Package component — vision media enhancement: enriches parsed JSON items
// with local OCR and vision-model descriptions of embedded images/tables.
// Mirrors Python's enhance_media_sections_with_vision
// (rag/flow/parser/utils.py:162, called at parser.py:772/978/1115).

package component

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdflayout "ragflow/internal/deepdoc/parser/pdf/layout"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

var (
	figureVisionPromptBuilder func(language string) (string, error) = buildFigureVisionPrompt
	visionChatInvoker                                               = defaultVisionChatInvoker
	visionImageCropperFactory                                       = newVisionImageCropper
)

const (
	figureVisionPromptFile           = "vision_llm_figure_describe_prompt.md"
	visionEnhancementConcurrency int = 10
	// visionChatTimeout bounds a single VLM call so a hung endpoint cannot
	// occupy one of the concurrency slots indefinitely. Python wraps the
	// per-image call in @timeout(30, 3) (deepdoc/parser/figure_parser.py).
	visionChatTimeout = 30 * time.Second
)

var (
	visionOCRInvokeBudget = 60 * time.Second
	visionOCRItemBudget   = 10 * time.Second
)

var (
	figureVisionPrompts     promptDirState
	figureVisionPromptCache = make(map[string]string)
	figureVisionPromptMu    sync.RWMutex
)

// promptDirState caches the project root that contains rag/prompts. The once
// guard makes initialization sticky; unlike a function-local initErr, an
// initialization failure is preserved for every subsequent call.
type promptDirState struct {
	once    sync.Once
	root    string
	initErr error
}

func (s *promptDirState) resolve(requestedRoot string) (string, error) {
	s.once.Do(func() {
		if info, statErr := os.Stat(filepath.Join(requestedRoot, "rag", "prompts")); statErr == nil && info.IsDir() {
			s.root = requestedRoot
			return
		}
		s.initErr = fmt.Errorf("rag/prompts not found under project root %q", requestedRoot)
	})
	if s.initErr != nil {
		return "", s.initErr
	}
	if s.root == "" {
		return "", errors.New("prompts base dir not initialized")
	}
	return s.root, nil
}

// isUsableVisionImage reports whether raw carries a valid image data URI or a
// base64 payload that can be sent to a vision model.
func isUsableVisionImage(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if len(raw) > maxVLMEncodedBytes+256 {
		return false
	}
	if strings.HasPrefix(raw, "data:image/") {
		idx := strings.Index(raw, "base64,")
		if idx < 0 {
			return false
		}
		return isValidBase64(raw[idx+len("base64,"):], false)
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return true
	}
	if len(raw) > maxVLMEncodedBytes {
		return false
	}
	return isValidBase64(raw, true)
}

func isValidBase64(s string, stripWhitespace bool) bool {
	if s == "" || len(s) > maxVLMEncodedBytes {
		return false
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		var source io.Reader = strings.NewReader(s)
		if stripWhitespace {
			source = &visionPayloadReader{source: s}
		}
		decoded := base64.NewDecoder(encoding, source)
		n, err := io.CopyN(io.Discard, decoded, int64(maxVisionImageBytes)+1)
		if n > int64(maxVisionImageBytes) {
			return false
		}
		if err == io.EOF {
			return n > 0
		}
	}
	return false
}

// visionImageCropper materializes the raster and VLM payload for a parsed
// item. Under cgo it crops PDF sections on demand; both build variants use
// inline images directly. Close releases any re-acquired native engine.
type visionImageCropper interface {
	Crop(ctx context.Context, item map[string]any) (*visionImage, error)
	Close() error
}

type visionImage struct {
	Raster           image.Image
	VLMData          string
	VLMDataValidated bool
}

// maybeDispatchVisionEnhancement appends local OCR and VLM descriptions to
// parsed image resources and table regions.
// Mirrors Python's enhance_media_sections_with_vision in rag/flow/parser/utils.py:162.
func maybeDispatchVisionEnhancement(
	ctx context.Context,
	db *gorm.DB,
	fileType utility.FileType,
	dispatched parser.ParseResult,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parser.ParseResult, bool, error) {
	// Only enhance successful JSON output format containing items.
	if dispatched.Err != nil || dispatched.OutputFormat != "json" || len(dispatched.JSON) == 0 {
		return dispatched, false, nil
	}

	tenantID := getStringOr(inputs, "tenant_id", "")
	family := resolveParserFamily(fileType)
	setup := setups[family]
	language := resolveVisionLanguage(inputs, getStringOr(setup, "lang", ""))

	// Collect visual resources. A table without an inline image or PDF crop
	// locator is structured text, not an OCR target.
	var items []int
	for i, item := range dispatched.JSON {
		kd, _ := item["doc_type_kwd"].(string)
		if kd != "image" && kd != "table" {
			continue
		}
		if img, _ := item["image"].(string); img != "" {
			items = append(items, i)
			continue
		}
		if _, ok := parser.ExtractPDFPositions(item); ok {
			items = append(items, i)
		}
	}
	if len(items) == 0 {
		return dispatched, false, nil
	}
	ocrCtx, cancelOCR := context.WithTimeout(ctx, visionOCRInvokeBudget)
	defer cancelOCR()

	// Resolve VLM independently from local OCR. OCR still runs when the tenant
	// has no configured vision model.
	var driver modelModule.ModelDriver
	var modelName string
	var apiConfig *modelModule.APIConfig
	var prompt string
	vlmReady := false
	if tenantID != "" {
		modelRef := configuredMediaModelID(setup, family)
		var err error
		if modelRef != "" {
			driver, modelName, apiConfig, _, err = resolveModelConfig(ctx, db, tenantID, entity.ModelTypeImage2Text, modelRef)
			if err != nil {
				common.Warn("vision enhancement: per-call VLM resolve failed, falling back to tenant default",
					zap.String("family", family), zap.String("modelRef", modelRef), zap.String("tenant", tenantID), zap.Error(err))
				driver, modelName, apiConfig, _, err = resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeImage2Text)
			}
		} else {
			driver, modelName, apiConfig, _, err = resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeImage2Text)
		}
		if err == nil {
			prompt, err = figureVisionPromptBuilder(language)
			vlmReady = err == nil
		}
	}

	// Materialize one resource at a time. The VLM semaphore is acquired before
	// materialization, so at most visionEnhancementConcurrency encoded payloads
	// remain live while model requests run.
	cropper, cerr := visionImageCropperFactory(ctx, db, inputs)
	if cerr != nil {
		return dispatched, false, nil
	}
	defer cropper.Close()
	modified := false
	parseMethod := getStringOr(setup, "parse_method", "")
	descriptions := make([]string, len(items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, visionEnhancementConcurrency)
	for slot, itemIdx := range items {
		if err := ctx.Err(); err != nil {
			break
		}
		if ocrCtx.Err() != nil && !vlmReady {
			break
		}
		vlmSlot := false
		if vlmReady {
			select {
			case sem <- struct{}{}:
				vlmSlot = true
			case <-ctx.Done():
				break
			}
			if !vlmSlot {
				break
			}
		}
		var resource *visionImage
		func() {
			mediaParent := ctx
			if !vlmReady {
				mediaParent = ocrCtx
			}
			itemCtx, cancelItem := context.WithTimeout(mediaParent, visionOCRItemBudget)
			defer cancelItem()
			release, err := sharedOCRMediaAdmission().acquire(itemCtx)
			if err != nil {
				if ctx.Err() == nil && vlmReady {
					if payload, _ := dispatched.JSON[itemIdx]["image"].(string); payload != "" {
						resource = &visionImage{VLMData: payload}
					}
				}
				return
			}
			defer release()
			item := dispatched.JSON[itemIdx]
			resource, err = cropper.Crop(itemCtx, item)
			if err != nil || resource == nil {
				if ctx.Err() == nil && vlmReady {
					if payload, _ := item["image"].(string); isUsableVisionImage(payload) {
						resource = &visionImage{VLMData: payload, VLMDataValidated: true}
					}
				}
				return
			}
			if imageWithinOCRLimits(resource.Raster) && mediaOCRStatus(fileType, parseMethod, dispatched.JSON[itemIdx]) == ocrPending && ocrCtx.Err() == nil {
				ocrItemCtx, cancelOCRItem := context.WithTimeout(ocrCtx, visionOCRItemBudget)
				text, ocrErr := runLocalImageOCRImage(ocrItemCtx, resource.Raster)
				cancelOCRItem()
				if ocrErr == nil && strings.TrimSpace(text) != "" {
					appendItemText(dispatched.JSON[itemIdx], strings.TrimSpace(text))
					modified = true
				}
			}
			if ctx.Err() == nil && vlmReady && resource.VLMData == "" && resource.Raster != nil {
				resource.VLMData, err = encodeVisionRaster(resource.Raster)
				if err != nil {
					resource.VLMData = ""
				} else {
					resource.VLMDataValidated = resource.VLMData != ""
				}
			}
		}()
		if err := ctx.Err(); err != nil {
			if vlmSlot {
				<-sem
			}
			break
		}
		if resource == nil {
			if vlmSlot {
				<-sem
			}
			continue
		}
		if !vlmSlot || (!resource.VLMDataValidated && !isUsableVisionImage(resource.VLMData)) {
			if vlmSlot {
				<-sem
			}
			continue
		}
		wg.Add(1)
		go func(slot int, imageData string) {
			defer wg.Done()
			defer func() { <-sem }()

			messages := buildVisionMessages(prompt, imageData)
			if len(messages) == 0 {
				return
			}
			resp, ierr := visionChatInvoker(ctx, driver, modelName, messages, apiConfig)
			if ierr != nil {
				return
			}
			descriptions[slot] = extractVisionAnswer(resp)
		}(slot, resource.VLMData)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return dispatched, modified, err
	}

	// Append descriptions after any OCR text (single newline \n, matching Python).
	for slot, itemIdx := range items {
		desc := strings.TrimSpace(descriptions[slot])
		if desc == "" {
			continue
		}
		appendItemText(dispatched.JSON[itemIdx], desc)
		modified = true
	}

	return dispatched, modified, nil
}

type ocrStatus int

const (
	ocrPending ocrStatus = iota
	ocrAttempted
	ocrUnknown
)

func mediaOCRStatus(fileType utility.FileType, parseMethod string, item map[string]any) ocrStatus {
	if marker, ok := item["ocr_status_kwd"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(marker)) {
		case "pending":
			return ocrPending
		case "attempted":
			return ocrAttempted
		case "unknown":
			return ocrUnknown
		}
	}
	if fileType != utility.FileTypePDF {
		return ocrPending
	}
	method := strings.TrimSpace(parseMethod)
	if method == "" {
		method = "deepdoc"
	}
	if !strings.EqualFold(method, "deepdoc") {
		return ocrUnknown
	}
	if kind, _ := item["doc_type_kwd"].(string); kind == "table" {
		return ocrAttempted
	}
	return ocrPending
}

func appendItemText(item map[string]any, text string) {
	existing, _ := item["text"].(string)
	if existing == "" {
		item["text"] = text
		return
	}
	item["text"] = existing + "\n" + text
}

func buildFigureVisionPrompt(language string) (string, error) {
	template, err := loadFigureVisionPromptFile(figureVisionPromptFile)
	if err != nil {
		return "", err
	}
	return renderFigureVisionLanguage(template, language), nil
}

func loadFigureVisionPromptFile(filename string) (string, error) {
	figureVisionPromptMu.RLock()
	if cached, ok := figureVisionPromptCache[filename]; ok {
		figureVisionPromptMu.RUnlock()
		return cached, nil
	}
	figureVisionPromptMu.RUnlock()

	baseDir, err := figureVisionPromptsBaseDir()
	if err != nil {
		return "", err
	}
	promptPath := filepath.Join(baseDir, "rag", "prompts", filename)
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("figure vision prompt %q: %w", filename, err)
	}
	cached := strings.TrimSpace(string(content))
	figureVisionPromptMu.Lock()
	figureVisionPromptCache[filename] = cached
	figureVisionPromptMu.Unlock()
	return cached, nil
}

func figureVisionPromptsBaseDir() (string, error) {
	return figureVisionPrompts.resolve(utility.GetProjectRoot())
}

func buildVisionMessages(prompt, imageBase64 string) []modelModule.Message {
	dataURI := pdflayout.InlinePNGDataURL(imageBase64)
	if dataURI == "" {
		return nil
	}
	return []modelModule.Message{{
		Role: "user",
		Content: []interface{}{
			map[string]any{"type": "text", "text": prompt},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
		},
	}}
}

// extractVisionAnswer strips ```markdown fences from the VLM response.
// Python calls clean_markdown_block inside vision_llm_chunk (rag/app/picture.py:198,
// i.e., inside the invoker wrapper). Go applies it here post-invoker so that
// visionChatInvoker stays a pure, swappable test seam — output is equivalent.
func extractVisionAnswer(resp *modelModule.ChatResponse) string {
	if resp == nil || resp.Answer == nil {
		return ""
	}
	return common.CleanMarkdownBlock(*resp.Answer)
}

func defaultVisionChatInvoker(
	ctx context.Context,
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	chatCtx, cancel := context.WithTimeout(ctx, visionChatTimeout)
	defer cancel()
	vision := true
	config := &modelModule.ChatConfig{Vision: &vision}
	if _, ok := driver.(*modelModule.OllamaModel); ok {
		thinking := false
		config.Thinking = &thinking
	}
	return driver.ChatWithMessages(chatCtx, modelName, messages, apiConfig, config, nil)
}
