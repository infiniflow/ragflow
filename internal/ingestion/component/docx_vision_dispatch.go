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
// item carries a doc_type_kwd and an optional image. The markdown path
// performs no vision enrichment in Python, so it must not here either.

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
)

var (
	docxVisionPromptBuilder      = buildDOCXVisionPrompt
	visionChatInvoker            = defaultVisionChatInvoker
	docxVisionConcurrency   uint = 10
)

const (
	docxVisionPromptFile            = "vision_llm_figure_describe_prompt.md"
	docxVisionPromptWithContextFile = "vision_llm_figure_describe_prompt_with_context.md"
)

var (
	docxVisionPromptsBase string
	docxVisionPromptsOnce sync.Once
	docxVisionPromptCache = make(map[string]string)
	docxVisionPromptMu    sync.RWMutex
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
// The markdown output path receives no vision enrichment — Python's DOCX
// markdown branch only concatenates text and never calls the vision model.
func maybeDispatchDOCXVision(
	ctx context.Context,
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

	// Resolve the tenant's IMAGE2TEXT model.
	driver, modelName, apiConfig, _, err := resolveTenantModelByType(tenantID, entity.ModelTypeImage2Text)
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
			// former markdown path), so use the bare figure prompt —
			// matching Python's VisionFigureParser(context_size=0).
			prompt, perr := docxVisionPromptBuilder("", "")
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

// buildDOCXVisionPrompt loads the figure-describe prompt template
// and, when context text is available, renders it with the
// with-context variant. Mirrors Python:
//
//	if context_above or context_below:
//	    prompt = vision_llm_figure_describe_prompt_with_context(context_above, context_below)
//	else:
//	    prompt = vision_llm_figure_describe_prompt()
func buildDOCXVisionPrompt(contextAbove, contextBelow string) (string, error) {
	hasContext := strings.TrimSpace(contextAbove) != "" || strings.TrimSpace(contextBelow) != ""

	var templateName string
	if hasContext {
		templateName = docxVisionPromptWithContextFile
	} else {
		templateName = docxVisionPromptFile
	}

	template, err := loadDOCXVisionPromptFile(templateName)
	if err != nil {
		return "", err
	}

	if hasContext {
		template = strings.ReplaceAll(template, "{{ context_above }}", contextAbove)
		template = strings.ReplaceAll(template, "{{ context_below }}", contextBelow)
	}
	return template, nil
}

func loadDOCXVisionPromptFile(filename string) (string, error) {
	docxVisionPromptMu.RLock()
	if cached, ok := docxVisionPromptCache[filename]; ok {
		docxVisionPromptMu.RUnlock()
		return cached, nil
	}
	docxVisionPromptMu.RUnlock()

	baseDir, err := docxVisionPromptsBaseDir()
	if err != nil {
		return "", err
	}
	promptPath := filepath.Join(baseDir, "rag", "prompts", filename)
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("docx vision prompt %q: %w", filename, err)
	}
	cached := strings.TrimSpace(string(content))
	docxVisionPromptMu.Lock()
	docxVisionPromptCache[filename] = cached
	docxVisionPromptMu.Unlock()
	return cached, nil
}

func docxVisionPromptsBaseDir() (string, error) {
	var initErr error
	docxVisionPromptsOnce.Do(func() {
		root := utility.GetProjectRoot()
		if _, statErr := os.Stat(filepath.Join(root, "rag", "prompts")); statErr == nil {
			docxVisionPromptsBase = root
			return
		}
		initErr = fmt.Errorf("rag/prompts not found under project root %q", root)
	})
	if initErr != nil {
		return "", initErr
	}
	return docxVisionPromptsBase, nil
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
