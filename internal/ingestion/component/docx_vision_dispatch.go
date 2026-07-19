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
// Python's vision_figure_parser_docx_wrapper_naive in
// deepdoc/parser/figure_parser.py.
//
// Unlike the PDF vision path (which replaces dispatchParse
// entirely), DOCX vision is a post-processing step: it takes
// the already-parsed markdown + extracted figures and augments
// the markdown text with vision model descriptions.

package component

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// maybeDispatchDOCXVision checks whether the dispatch result for a
// DOCX file contains embedded image figures and, when a vision
// model is available, enriches the markdown with AI-generated
// figure descriptions. It mirrors the Python flow:
//
//  1. naive_merge_docx  → chunks (text + images + context)
//  2. vision_figure_parser_docx_wrapper_naive → LLM descriptions
//
// The function is called AFTER dispatchParse so the normal parse
// path produces figures in dispatched.File["figures"].
// It returns (result, handled, error). handled is true when the
// dispatched result was modified.
func maybeDispatchDOCXVision(
	fileType utility.FileType,
	dispatched parserDispatchResult,
	inputs map[string]any,
	setups map[string]schema.ParserSetup,
) (parserDispatchResult, bool, error) {
	if fileType != utility.FileTypeDOCX {
		return dispatched, false, nil
	}
	if dispatched.Err != nil || dispatched.OutputFormat != "markdown" {
		return dispatched, false, nil
	}
	figs, hasFigures := extractDOCXFiguresFromDispatch(dispatched)
	if !hasFigures {
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

	descriptions := make([]string, len(figs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, docxVisionConcurrency)

	for i, fig := range figs {
		wg.Add(1)
		go func(idx int, f map[string]any) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			imageB64, _ := f["image"].(string)
			ctxAbove, _ := f["context_above"].(string)
			ctxBelow, _ := f["context_below"].(string)

			if imageB64 == "" {
				return
			}

			prompt, err := docxVisionPromptBuilder(ctxAbove, ctxBelow)
			if err != nil {
				return
			}

			messages := buildVisionMessages(prompt, imageB64)
			resp, err := visionChatInvoker(driver, modelName, messages, apiConfig)
			if err != nil {
				return
			}
			descriptions[idx] = extractDOCXVisionAnswer(resp)
		}(i, fig)
	}
	wg.Wait()

	// Insert each description at the figure's position in the markdown,
	// matching Python's `chunks[idx]["text"] += description`.
	// Figures carry a "marker" (text immediately before the image) to
	// locate the insertion point. Process in reverse order so earlier
	// insertions don't shift later markers.
	md := dispatched.Markdown
	type indexedDesc struct {
		idx  int
		desc string
	}
	var inserts []indexedDesc
	for i, d := range descriptions {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		if i >= len(figs) {
			continue
		}
		marker, _ := figs[i]["marker"].(string)
		if marker != "" {
			if pos := strings.LastIndex(md, marker); pos >= 0 {
				inserts = append(inserts, indexedDesc{idx: pos + len(marker), desc: d})
				continue
			}
		}
		// Fallback: try context_above as a search anchor.
		if ctx, _ := figs[i]["context_above"].(string); ctx != "" {
			if pos := strings.LastIndex(md, ctx); pos >= 0 {
				inserts = append(inserts, indexedDesc{idx: pos + len(ctx), desc: d})
				continue
			}
		}
		// No anchor found — append to end.
		inserts = append(inserts, indexedDesc{idx: len(md), desc: "\n\n" + d})
	}
	// Sort descending by position for stable insertion.
	sort.Slice(inserts, func(a, b int) bool { return inserts[a].idx > inserts[b].idx })
	for _, ins := range inserts {
		desc := ins.desc
		if !strings.HasPrefix(desc, "\n") {
			desc = "\n\n" + desc
		}
		md = md[:ins.idx] + desc + md[ins.idx:]
	}
	dispatched.Markdown = md

	return dispatched, true, nil
}

func extractDOCXFiguresFromDispatch(dispatched parserDispatchResult) ([]map[string]any, bool) {
	if dispatched.File == nil {
		return nil, false
	}
	raw, ok := dispatched.File["figures"]
	if !ok {
		return nil, false
	}
	list, ok := raw.([]map[string]any)
	if !ok {
		return nil, false
	}
	if len(list) == 0 {
		return nil, false
	}
	return list, true
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
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	vision := true
	return driver.ChatWithMessages(modelName, messages, apiConfig, &modelModule.ChatConfig{Vision: &vision}, nil)
}
