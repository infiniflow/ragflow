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

package chat

import (
	"slices"
	"strings"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

// Message fitting (Python message_fit_in).
//
// This lives in the chat package rather than internal/agent/component so that
// both component (which owns the production eino invoker) and
// internal/rag/advanced_rag/harness (which composes answers) can fit prompts
// without forming an import cycle — component cannot import harness, because
// harness's own tests import component.

// EffectiveContextLength returns maxLength if positive, otherwise 8192.
// Mirrors Python's LLM.effective_context_length in PR #16413 — prevents
// zero/negative context windows from silently trimming all prompt content.
func EffectiveContextLength(maxLength int) int {
	if maxLength > 0 {
		return maxLength
	}
	return 8192
}

// ContextFitBudget returns 97% of the effective context length as the
// token budget for message_fit_in. Mirrors Python's LLM.context_fit_budget
// in PR #16413.
func ContextFitBudget(maxLength int) int {
	return int(float64(EffectiveContextLength(maxLength)) * 0.97)
}

// validateFittedMessages checks that the fitted message list is non-empty
// and the last message is a non-empty user turn (content or multi-modal
// parts). Returns an error string on failure, empty string on success.
// Python requires len >= 2 because the system prompt is always injected
// upstream; Go allows len >= 1 because the system message may be embedded
// inside msgs (from buildMessagesWithImages) or absent entirely.
func validateFittedMessages(msgFit []schema.Message) string {
	if len(msgFit) == 0 {
		return "**ERROR**: message_fit_in produced insufficient messages for LLM"
	}
	last := msgFit[len(msgFit)-1]
	if last.Role != schema.User {
		return "**ERROR**: LLM last message is not a user turn after prompt fitting; check model content_length context setting"
	}
	if strings.TrimSpace(last.Content) == "" && len(last.UserInputMultiContent) == 0 {
		return "**ERROR**: LLM user message is empty after prompt fitting; check model content_length context setting"
	}
	return ""
}

// FitMessages applies message_fit_in semantics to the given messages and
// validates that the result ends with a non-empty user turn. Returns the
// fitted messages and an error string (empty on success).
// Mirrors Python's LLM.fit_messages in PR #16413.
func FitMessages(systemPrompt string, msgs []schema.Message, maxLength int) ([]schema.Message, string) {
	// Deep-copy msgs (mirrors Python's deepcopy) to avoid mutating caller's slice.
	copied := make([]schema.Message, len(msgs))
	for i, m := range msgs {
		cloned := slices.Clone(m.UserInputMultiContent)
		for j, p := range cloned {
			if p.Image != nil {
				imgCopy := *p.Image
				if p.Image.URL != nil {
					u := *p.Image.URL
					imgCopy.URL = &u
				}
				cloned[j].Image = &imgCopy
			}
		}
		copied[i] = schema.Message{
			Role:                  m.Role,
			Content:               m.Content,
			UserInputMultiContent: cloned,
		}
	}

	// Convert to messagefit.Message. Track where each entry's text lives
	// (plain Content or a multi-modal text part) so the fitted text can be
	// written back to the right field. Entries with no text at all
	// (image-only turns) carry an empty Content in messagefit and survive
	// fitting when kept.
	type fitSource struct {
		copiedIdx     int  // index into copied; -1 for the synthetic system prompt
		multiIdx      int  // -1 means the text lives in Content
		textInContent bool // the original message carried text in Content
	}
	all := make([]tokenizer.Message, 0, 1+len(copied))
	sources := make([]fitSource, 0, 1+len(copied))

	if systemPrompt != "" {
		all = append(all, tokenizer.Message{Role: "system", Content: systemPrompt})
		sources = append(sources, fitSource{copiedIdx: -1, multiIdx: 0})
	}

	for i := range copied {
		text := copied[i].Content
		multiIdx := -1
		hadText := text != ""
		if !hadText {
			// Fold every non-empty text part into the token budget: only the
			// first text part is written back, so leaving later parts out
			// would let text exceed the fitted budget after reconstruction.
			var textParts []string
			for j, p := range copied[i].UserInputMultiContent {
				if p.Type == schema.ChatMessagePartTypeText && p.Text != "" {
					textParts = append(textParts, p.Text)
					if multiIdx < 0 {
						multiIdx = j
					}
				}
			}
			if len(textParts) > 0 {
				text = strings.Join(textParts, "\n\n")
			}
		}
		all = append(all, tokenizer.Message{Role: string(copied[i].Role), Content: text})
		sources = append(sources, fitSource{copiedIdx: i, multiIdx: multiIdx, textInContent: copied[i].Content != ""})
	}

	// Use 97% of effective context as the token budget.
	budget := ContextFitBudget(maxLength)
	kept, keptIdx, _ := tokenizer.Fit(all, budget)

	// Convert back to []schema.Message. messagefit.Fit reports exactly which
	// entries are kept (keptIdx); dropped entries are simply absent, so no
	// empty-content sentinel is needed and image-only turns are preserved.
	result := make([]schema.Message, 0, len(kept))
	for j, i := range keptIdx {
		src := sources[i]
		if src.copiedIdx < 0 {
			result = append(result, schema.Message{Role: schema.System, Content: kept[j].Content})
			continue
		}
		m := copied[src.copiedIdx]
		if src.multiIdx >= 0 && src.multiIdx < len(m.UserInputMultiContent) {
			m.UserInputMultiContent[src.multiIdx].Text = kept[j].Content
			// Drop any additional text parts: their content was folded into
			// the first part before fitting, so keeping them would re-introduce
			// text outside the token budget.
			keptParts := m.UserInputMultiContent[:0]
			for k, part := range m.UserInputMultiContent {
				if part.Type == schema.ChatMessagePartTypeText && k != src.multiIdx {
					continue
				}
				keptParts = append(keptParts, part)
			}
			m.UserInputMultiContent = keptParts
		} else if src.textInContent {
			// Always write the fitted text back (even when trimmed to empty):
			// leaving the original would send untrimmed content past the budget.
			m.Content = kept[j].Content
		}
		result = append(result, m)
	}
	return result, validateFittedMessages(result)
}
