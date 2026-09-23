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

// Package component — vision figure enhancement: enriches parsed JSON items
// (PDF / DOCX / Markdown, OutputFormat=="json") with vision-model descriptions
// of embedded images/tables. Mirrors Python's enhance_media_sections_with_vision
// (rag/flow/parser/utils.py:162, called at parser.py:772/978/1115).

package component

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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
	if strings.HasPrefix(raw, "data:image/") {
		idx := strings.Index(raw, "base64,")
		if idx < 0 {
			return false
		}
		return isValidBase64(raw[idx+len("base64,"):])
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return true
	}
	cleaned := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	return isValidBase64(cleaned)
}

func isValidBase64(s string) bool {
	if s == "" {
		return false
	}
	if _, err := base64.StdEncoding.DecodeString(s); err == nil {
		return true
	}
	_, err := base64.RawStdEncoding.DecodeString(s)
	return err == nil
}

// visionImageCropper yields a vision-usable base64 image for a parsed item.
// Under cgo it crops on demand from the source PDF when the item carries
// positions but no inlined image; under !cgo it returns the inlined image
// (the only form available without a native renderer). Close releases any
// re-acquired engine so native handles are not leaked.
type visionImageCropper interface {
	Crop(item map[string]any) (string, error)
	Close() error
}

// maybeDispatchVisionEnhancement enriches parsed JSON items with vision-model
// descriptions of embedded images and tables (doc_type_kwd in {"image", "table"}
// with non-empty image field).
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
	if tenantID == "" {
		return dispatched, false, nil
	}
	// Language priority mirrors Python's enhance_media_sections_with_vision
	// (rag/flow/parser/parser.py:778): the run-level dataset language first
	// (the Parser pulls it into inputs from Globals), then the family setup's
	// lang, then English.
	family := resolveParserFamily(fileType)
	setup := setups[family]
	language := resolveVisionLanguage(inputs, getStringOr(setup, "lang", ""))

	// 1. Collect target items (images/tables that can be described). A target
	// may carry an inlined image (docx/markdown, or any pre-inlined source) or,
	// under cgo for PDF, only PDF positions — in which case the cropper
	// re-acquires the source PDF and crops on demand (the parser no longer
	// inlines PDF media). The cropper transparently handles both forms.
	type target struct {
		idx int
	}
	var targets []target
	for i, item := range dispatched.JSON {
		kd, _ := item["doc_type_kwd"].(string)
		if kd != "image" && kd != "table" {
			continue
		}
		if img, _ := item["image"].(string); img != "" {
			targets = append(targets, target{idx: i})
			continue
		}
		if _, ok := parser.ExtractPDFPositions(item); ok {
			targets = append(targets, target{idx: i})
		}
	}
	if len(targets) == 0 {
		return dispatched, false, nil
	}

	// Acquire the on-demand cropper (cgo: crops from storage; !cgo: returns
	// the inlined image). Best-effort: a failure here means no vision
	// enhancement, matching Python's try/except pass.
	cropper, cerr := newVisionImageCropper(ctx, db, inputs)
	if cerr != nil {
		return dispatched, false, nil
	}
	defer cropper.Close()

	// 2. Resolve the per-call IMAGE2TEXT model, then fall back to the tenant
	// default. Mirror Python's vlm_conf["llm_id"] preference.
	modelRef := configuredMediaModelID(setup, family)
	var driver modelModule.ModelDriver
	var modelName string
	var apiConfig *modelModule.APIConfig
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
	if err != nil {
		// Model not available — skip vision enhancement silently, matching Python's try/except pass.
		return dispatched, false, nil
	}

	// Hoist prompt once: language is invariant across items.
	prompt, perr := figureVisionPromptBuilder(language)
	if perr != nil {
		//nolint:nilerr // Vision enhancement is best-effort.
		return dispatched, false, nil
	}

	// 3. Concurrently invoke VLM — acquire semaphore before launching goroutine
	// so live goroutine count is bounded by visionEnhancementConcurrency.
	// Stop scheduling new VLM calls after cancellation; already-running calls
	// finish via wg.Wait() and cancellation propagates via ctx through
	// visionChatInvoker.
	descriptions := make([]string, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, visionEnhancementConcurrency)

dispatch:
	for slot, tg := range targets {
		if ctx.Err() != nil {
			break dispatch
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break dispatch
		}
		wg.Add(1)
		go func(slot int, itemIdx int) {
			defer wg.Done()
			defer func() { <-sem }()

			img, ierr := cropper.Crop(dispatched.JSON[itemIdx])
			if ierr != nil || img == "" {
				return
			}
			if !isUsableVisionImage(img) {
				common.Warn("vision enhancement: invalid image data skipped",
					zap.Int("item", itemIdx))
				return
			}
			messages := buildVisionMessages(prompt, img)
			if len(messages) == 0 {
				return
			}
			resp, ierr := visionChatInvoker(ctx, driver, modelName, messages, apiConfig)
			if ierr != nil {
				return
			}
			descriptions[slot] = extractVisionAnswer(resp)
		}(slot, tg.idx)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return dispatched, false, err
	}

	// 4. Append descriptions to item text (single newline \n, matching Python).
	modified := false
	for slot, tg := range targets {
		desc := strings.TrimSpace(descriptions[slot])
		if desc == "" {
			continue
		}
		existing, _ := dispatched.JSON[tg.idx]["text"].(string)
		if existing != "" {
			dispatched.JSON[tg.idx]["text"] = existing + "\n" + desc
		} else {
			dispatched.JSON[tg.idx]["text"] = desc
		}
		modified = true
	}

	return dispatched, modified, nil
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
