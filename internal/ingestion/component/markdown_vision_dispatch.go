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

// Markdown vision figure dispatch: enriches parsed markdown JSON
// items with LLM-generated descriptions of embedded images,
// mirroring Python's enhance_media_sections_with_vision in
// rag/flow/parser/utils.py, called from the _markdown path.
//
// Unlike the DOCX vision path (which processes a separate figures
// array), markdown vision iterates over the JSON items produced by
// MarkdownParser.ParseWithResult and enhances items whose
// doc_type_kwd == "image" and whose "image" field contains a
// base64-encoded image.

package component

import (
	"fmt"
	"strings"
	"sync"

	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

var (
	markdownVisionConcurrency uint = 10
)

// maybeDispatchMarkdownVision checks whether the markdown parse result
// contains JSON items with embedded images and, when a vision model is
// available, enriches those items with AI-generated figure descriptions.
//
// Mirrors the Python flow:
//
//  1. _markdown → sections + section_images (parser.py:1005)
//  2. enhance_media_sections_with_vision (parser.py:1054)
//
// The function is called AFTER dispatchParse so the normal parse
// path produces JSON items with doc_type_kwd == "image" and an
// "image" base64 field. It returns (result, handled, error).
func maybeDispatchMarkdownVision(
	fileType utility.FileType,
	dispatched parserDispatchResult,
	inputs map[string]any,
) (parserDispatchResult, bool, error) {
	if fileType != utility.FileTypeMarkdown {
		return dispatched, false, nil
	}
	if dispatched.Err != nil || dispatched.OutputFormat != "json" {
		return dispatched, false, nil
	}
	if len(dispatched.JSON) == 0 {
		return dispatched, false, nil
	}

	// Collect indices of image items.
	type imgItem struct {
		idx      int
		imageB64 string
		text     string
	}
	var images []imgItem
	for i, item := range dispatched.JSON {
		kd, _ := item["doc_type_kwd"].(string)
		if kd != "image" {
			continue
		}
		img, _ := item["image"].(string)
		if img == "" {
			continue
		}
		text, _ := item["text"].(string)
		images = append(images, imgItem{idx: i, imageB64: img, text: text})
	}
	if len(images) == 0 {
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

	descriptions := make([]string, len(images))
	var wg sync.WaitGroup
	sem := make(chan struct{}, markdownVisionConcurrency)

	for i, img := range images {
		wg.Add(1)
		go func(pos int, item imgItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Markdown images have no context — use the
			// default (no-context) prompt template.
			prompt, err := buildMarkdownVisionPrompt()
			if err != nil {
				return
			}

			messages := buildVisionMessages(prompt, item.imageB64)
			resp, err := visionChatInvoker(driver, modelName, messages, apiConfig)
			if err != nil {
				return
			}
			descriptions[pos] = extractDOCXVisionAnswer(resp)
		}(i, img)
	}
	wg.Wait()

	// Append vision descriptions to each image item's text field,
	// matching Python's `item["text"] = f"{text}\n{parsed_text}"`.
	for pos, img := range images {
		desc := strings.TrimSpace(descriptions[pos])
		if desc == "" {
			continue
		}
		item := dispatched.JSON[img.idx]
		existing, _ := item["text"].(string)
		if existing != "" {
			item["text"] = existing + "\n\n" + desc
		} else {
			item["text"] = desc
		}
	}

	return dispatched, true, nil
}

// buildMarkdownVisionPrompt loads the default (no-context) figure
// describe prompt template, mirroring Python's
// vision_llm_figure_describe_prompt().
func buildMarkdownVisionPrompt() (string, error) {
	template, err := loadDOCXVisionPromptFile(docxVisionPromptFile)
	if err != nil {
		return "", fmt.Errorf("markdown vision prompt: %w", err)
	}
	return template, nil
}
