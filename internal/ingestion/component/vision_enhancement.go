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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	pdflayout "ragflow/internal/deepdoc/parser/pdf/layout"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
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
)

var (
	figureVisionPromptsBase string
	figureVisionPromptsOnce sync.Once
	figureVisionPromptCache = make(map[string]string)
	figureVisionPromptMu    sync.RWMutex
)

// cleanMarkdownBlock mirrors Python common/string_utils.py:clean_markdown_block
//
//	re.sub(r"^\s*```markdown\s*\n?", "", text)
//	re.sub(r"\n?\s*```\s*$", "", text)
//
// Matches Python without re.MULTILINE so ^/$ anchor only the whole text.
var (
	reMarkdownOpen  = regexp.MustCompile(`^\s*` + "```" + `markdown\s*\n?`)
	reMarkdownClose = regexp.MustCompile(`\n?\s*` + "```" + `\s*$`)
)

func cleanMarkdownBlock(s string) string {
	s = reMarkdownOpen.ReplaceAllString(s, "")
	s = reMarkdownClose.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// isVisionEnhancementAllowed mirrors Python's 3 call sites in
// rag/flow/parser/parser.py:772/978/1115 (PDF/DOCX/Markdown JSON branches).
func isVisionEnhancementAllowed(fileType utility.FileType) bool {
	switch fileType {
	case utility.FileTypePDF, utility.FileTypeDOCX, utility.FileTypeMarkdown:
		return true
	default:
		return false
	}
}

// maybeDispatchVisionEnhancement enriches parsed JSON items with vision-model
// descriptions of embedded images and tables (doc_type_kwd in {"image", "table"}
// with non-empty image field).
// Mirrors Python's enhance_media_sections_with_vision in rag/flow/parser/utils.py:162.
func maybeDispatchVisionEnhancement(
	ctx context.Context,
	db *gorm.DB,
	fileType utility.FileType,
	dispatched parserDispatchResult,
	inputs map[string]any,
) (parserDispatchResult, bool, error) {
	// 0. FileType allowlist guard.
	if !isVisionEnhancementAllowed(fileType) {
		return dispatched, false, nil
	}
	// Only enhance successful JSON output format containing items.
	if dispatched.Err != nil || dispatched.OutputFormat != "json" || len(dispatched.JSON) == 0 {
		return dispatched, false, nil
	}

	tenantID := getStringOr(inputs, "tenant_id", "")
	if tenantID == "" {
		return dispatched, false, nil
	}
	language := resolveVisionLanguage(inputs, "")

	// 1. Collect target items (images/tables with non-empty image data).
	type target struct {
		idx int
	}
	var targets []target
	for i, item := range dispatched.JSON {
		kd, _ := item["doc_type_kwd"].(string)
		if kd != "image" && kd != "table" {
			continue
		}
		img, _ := item["image"].(string)
		if img == "" {
			continue
		}
		targets = append(targets, target{idx: i})
	}
	if len(targets) == 0 {
		return dispatched, false, nil
	}

	// 2. Resolve tenant's IMAGE2TEXT model.
	driver, modelName, apiConfig, _, err := resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeImage2Text)
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

			img, _ := dispatched.JSON[itemIdx]["image"].(string)
			if img == "" {
				return
			}
			messages := buildVisionMessages(prompt, img)
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
	var initErr error
	figureVisionPromptsOnce.Do(func() {
		root := utility.GetProjectRoot()
		if _, statErr := os.Stat(filepath.Join(root, "rag", "prompts")); statErr == nil {
			figureVisionPromptsBase = root
			return
		}
		initErr = fmt.Errorf("rag/prompts not found under project root %q", root)
	})
	if initErr != nil {
		return "", initErr
	}
	return figureVisionPromptsBase, nil
}

func buildVisionMessages(prompt, imageBase64 string) []modelModule.Message {
	dataURI := pdflayout.InlinePNGDataURL(imageBase64)
	if dataURI == "" {
		dataURI = strings.TrimSpace(imageBase64)
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
	return cleanMarkdownBlock(*resp.Answer)
}

func defaultVisionChatInvoker(
	ctx context.Context,
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	vision := true
	return driver.ChatWithMessages(ctx, modelName, messages, apiConfig, &modelModule.ChatConfig{Vision: &vision}, nil)
}
