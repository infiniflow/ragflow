//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// DOCX vision figure dispatch: enriches the parse result with
// LLM-generated descriptions of embedded images, mirroring
// Python's enhance_media_sections_with_vision in
// rag/flow/parser/utils.py (invoked from parser.py:_doc's JSON branch).
//
// Unlike the PDF vision path (which replaces dispatchParse entirely),
// DOCX vision is a post-processing step. It mirrors Python exactly:
// vision enrichment happens ONLY on the JSON output path, where each
// item carries a doc_type_kwd and an optional image. The Markdown path
// performs no vision enrichment in Python, so it must not be here either.

package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

var (
	figureVisionPromptBuilder      = buildFigureVisionPrompt
	visionChatInvoker              = defaultVisionChatInvoker
	docxVisionConcurrency     uint = 10
)

const (
	figureVisionPromptFile            = "vision_llm_figure_describe_prompt.md"
	figureVisionPromptWithContextFile = "vision_llm_figure_describe_prompt_with_context.md"
)

var (
	figureVisionPromptsBase string
	figureVisionPromptsOnce sync.Once
	figureVisionPromptCache = make(map[string]string)
	figureVisionPromptMu    sync.RWMutex
)

// maybeDispatchDOCXVision enriches a DOCX parse result with vision-model
// descriptions of embedded images. It mirrors Python's
// enhance_media_sections_with_vision (rag/flow/parser/utils.py:162), which
// runs only in the JSON output branch of parser.py:_doc.
//
// For each JSON item whose doc_type_kwd is "image" or "table" AND that
// carries a non-empty "image" field, the vision model describes the image
// and the description is appended to the item's text (Python:
// item["text"] = f"{text}\n{parsed_text}" if text else parsed_text). Items
// without an image (e.g. DOCX tables) are left untouched, exactly as Python
// skips them via `if item.get("image") is None: continue`.
//
// The Markdown output path receives no vision enrichment — Python's DOCX
// Markdown branch only concatenates text and never calls the vision model.
func maybeDispatchDOCXVision(
	ctx context.Context,
	db *gorm.DB,
	fileType utility.FileType,
	dispatched parserDispatchResult,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parserDispatchResult, bool, error) {
	if fileType != utility.FileTypeDOCX {
		return dispatched, false, nil
	}
	// Python triggers vision enrichment only on the JSON path
	// (parser.py:_doc → enhance_media_sections_with_vision).
	if dispatched.Err != nil || dispatched.OutputFormat != "json" || len(dispatched.JSON) == 0 {
		return dispatched, false, nil
	}

	tenantID := getStringOr(inputs, "tenant_id", "")
	if tenantID == "" {
		return dispatched, false, nil
	}
	language := resolveVisionLanguage(inputs, "")

	// Resolve the tenant's IMAGE2TEXT model.
	driver, modelName, apiConfig, _, err := resolveTenantModelByType(ctx, db, tenantID, entity.ModelTypeImage2Text)
	if err != nil {
		// Model not available — skip vision enhancement silently,
		// matching Python's try/except pass behaviour.
		return dispatched, false, nil
	}

	// Collect the indices of JSON items that carry an embeddable image.
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

	descriptions := make([]string, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, docxVisionConcurrency)

	for slot, tg := range targets {
		wg.Add(1)
		go func(slot int, itemIdx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			img, _ := dispatched.JSON[itemIdx]["image"].(string)
			if img == "" {
				return
			}
			// DOCX JSON items have no surrounding context (unlike the
			// former Markdown path), so use the bare figure prompt —
			// matching Python's VisionFigureParser(context_size=0).
			prompt, perr := figureVisionPromptBuilder("", "", language)
			if perr != nil {
				return
			}
			messages := buildVisionMessages(prompt, img)
			resp, ierr := visionChatInvoker(ctx, driver, modelName, messages, apiConfig)
			if ierr != nil {
				return
			}
			descriptions[slot] = extractDOCXVisionAnswer(resp)
		}(slot, tg.idx)
	}
	wg.Wait()

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

// buildFigureVisionPrompt loads the figure-describe prompt template
// and, when context text is available, renders it with the
// with-context variant. Mirrors Python:
//
//	if context_above or context_below:
//	    prompt = vision_llm_figure_describe_prompt_with_context(
//	        context_above, context_below, language=language)
//	else:
//	    prompt = vision_llm_figure_describe_prompt(language=language)
func buildFigureVisionPrompt(contextAbove, contextBelow, language string) (string, error) {
	hasContext := strings.TrimSpace(contextAbove) != "" || strings.TrimSpace(contextBelow) != ""

	var templateName string
	if hasContext {
		templateName = figureVisionPromptWithContextFile
	} else {
		templateName = figureVisionPromptFile
	}

	template, err := loadFigureVisionPromptFile(templateName)
	if err != nil {
		return "", err
	}
	template = renderFigureVisionLanguage(template, language)

	if hasContext {
		template = strings.ReplaceAll(template, "{{ context_above }}", contextAbove)
		template = strings.ReplaceAll(template, "{{ context_below }}", contextBelow)
	}
	return template, nil
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
	dataURI := "data:image/png;base64," + imageBase64
	return []modelModule.Message{{
		Role: "user",
		Content: []interface{}{
			map[string]any{"type": "text", "text": prompt},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
		},
	}}
}

func extractDOCXVisionAnswer(resp *modelModule.ChatResponse) string {
	if resp == nil || resp.Answer == nil {
		return ""
	}
	return strings.TrimSpace(*resp.Answer)
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
