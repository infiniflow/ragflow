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
// and the last message is a user turn that still carries something usable —
// non-empty text, or a real media payload. Returns an error string on failure,
// empty string on success.
//
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
	if strings.TrimSpace(last.Content) == "" && !hasUsableContent(last.UserInputMultiContent) {
		return "**ERROR**: LLM user message is empty after prompt fitting; check model content_length context setting"
	}
	return ""
}

// hasUsableContent reports whether the multi-modal parts still give the model
// something to work with: non-empty text, or a media payload with a url or
// inline data. Parts merely EXISTING is not enough — a text-only turn whose
// text the fitting trimmed away leaves a non-empty slice holding an empty text
// part, and sending that turn would ask the model to answer nothing (Python's
// validate_fitted_messages rejects an empty user content the same way).
func hasUsableContent(parts []schema.MessageInputPart) bool {
	for _, p := range parts {
		if p.Type == schema.ChatMessagePartTypeText {
			if strings.TrimSpace(p.Text) != "" {
				return true
			}
			continue
		}
		for _, c := range payloadCommons(p) {
			hasURL := c.URL != nil && strings.TrimSpace(*c.URL) != ""
			hasData := c.Base64Data != nil && strings.TrimSpace(*c.Base64Data) != ""
			if hasURL || hasData {
				return true
			}
		}
	}
	return false
}

// payloadCommons returns the media payloads a part carries, if any.
func payloadCommons(p schema.MessageInputPart) []*schema.MessagePartCommon {
	out := make([]*schema.MessagePartCommon, 0, 4)
	if p.Image != nil {
		out = append(out, &p.Image.MessagePartCommon)
	}
	if p.Audio != nil {
		out = append(out, &p.Audio.MessagePartCommon)
	}
	if p.Video != nil {
		out = append(out, &p.Video.MessagePartCommon)
	}
	if p.File != nil {
		out = append(out, &p.File.MessagePartCommon)
	}
	return out
}

// FitMessages applies message_fit_in semantics to the given messages and
// validates that the result ends with a non-empty user turn. Returns the
// fitted messages and an error string (empty on success).
// Mirrors Python's LLM.fit_messages in PR #16413.
func FitMessages(systemPrompt string, msgs []schema.Message, maxLength int) ([]schema.Message, string) {
	// Deep-copy msgs (mirrors Python's deepcopy) to avoid mutating caller's slice.
	// Copy the WHOLE message first: the fitted history must keep every field the
	// model layer relies on (tool calls, tool ids, reasoning, extras, ...), which
	// a Role/Content/parts literal would silently drop. Then deep-copy only the
	// reference-backed fields fitting rewrites.
	copied := make([]schema.Message, len(msgs))
	for i, m := range msgs {
		cloned := m
		cloned.UserInputMultiContent = deepCopyInputParts(m.UserInputMultiContent)
		copied[i] = cloned
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

// deepCopyInputParts clones the multi-modal parts — the slice, the media
// payload structs and their url/base64 pointers — so fitting can rewrite a
// part's text or drop parts without touching the caller's message.
func deepCopyInputParts(parts []schema.MessageInputPart) []schema.MessageInputPart {
	out := slices.Clone(parts)
	for j, p := range out {
		if p.Image != nil {
			img := *p.Image
			img.MessagePartCommon = clonePartCommon(img.MessagePartCommon)
			out[j].Image = &img
		}
		if p.Audio != nil {
			audio := *p.Audio
			audio.MessagePartCommon = clonePartCommon(audio.MessagePartCommon)
			out[j].Audio = &audio
		}
		if p.Video != nil {
			video := *p.Video
			video.MessagePartCommon = clonePartCommon(video.MessagePartCommon)
			out[j].Video = &video
		}
		if p.File != nil {
			file := *p.File
			file.MessagePartCommon = clonePartCommon(file.MessagePartCommon)
			out[j].File = &file
		}
	}
	return out
}

// clonePartCommon copies a payload's pointer-backed fields so the fitted message
// never aliases the caller's payload.
func clonePartCommon(c schema.MessagePartCommon) schema.MessagePartCommon {
	if c.URL != nil {
		u := *c.URL
		c.URL = &u
	}
	if c.Base64Data != nil {
		d := *c.Base64Data
		c.Base64Data = &d
	}
	return c
}
