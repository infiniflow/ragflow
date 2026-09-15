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

package harness

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
)

// Ask the chat model to answer a question from a compiled-structure outline.
//
// Mirrors Python harness/structure_qa.py. Both Go navigation paths that read
// compiled rows render entities + relations into the same compact outline, ask
// the model whether that outline alone answers the question, and use the
// returned relevant_entities to pull the underlying source chunks even when the
// outline is NOT sufficient (so evidence still flows back to the caller).
//
// Lives apart from navigation.go because it is pure prompt/render work with no
// store access, and because both callers would otherwise import it from a file
// that in turn imports them.

// structureQATemperature pins Python structure_qa.py:79's
// {"temperature": 0.2} for the outline-verdict call.
const structureQATemperature = 0.2

// RenderStructure renders a compiled structure (entities + relations) as a
// compact outline for the prompt. Mirrors Python _render_structure:
//
//	Entities:
//	  - name (type): description
//	(blank line)
//	Relations:
//	  - src -[type]-> tgt
//
// Both lists are capped at maxStructureEntities / maxStructureRelations; empty
// names and empty relation endpoints are dropped. Entities with no type fall
// back to "other"; relations with no type fall back to "related".
func RenderStructure(entities, relations []map[string]any) string {
	var lines []string
	if len(entities) > 0 {
		lines = append(lines, "Entities:")
		n := len(entities)
		if n > maxStructureEntities {
			n = maxStructureEntities
		}
		for _, e := range entities[:n] {
			name := strings.TrimSpace(strAny(e["name"]))
			if name == "" {
				continue
			}
			typ := strings.TrimSpace(orStr(strAny(e["type"]), "other"))
			desc := strings.Join(strings.Fields(strAny(e["description"])), " ")
			line := "- " + name + " (" + typ + ")"
			if desc != "" {
				line += ": " + desc
			}
			lines = append(lines, line)
		}
	}
	if len(relations) > 0 {
		lines = append(lines, "\nRelations:")
		n := len(relations)
		if n > maxStructureRelations {
			n = maxStructureRelations
		}
		for _, r := range relations[:n] {
			src := strings.TrimSpace(strAny(r["from"]))
			tgt := strings.TrimSpace(strAny(r["to"]))
			if src == "" || tgt == "" {
				continue
			}
			rt := strings.TrimSpace(orStr(strAny(r["type"]), "related"))
			lines = append(lines, "- "+src+" -["+rt+"]-> "+tgt)
		}
	}
	return strings.Join(lines, "\n")
}

// AskStructure asks the chat model to answer `topic` from the rendered outline.
//
// Mirrors Python _ask_structure. Returns (answer, relevant_entity_names):
// answer is empty unless the model judged the outline sufficient; the names are
// always returned so the caller can pull the underlying source chunks. `noun`
// is the display noun ("catalog" / "mindmap" / "knowledge graph"); `label` is
// the log tag. `model` is the request-scoped chat model (Python passes
// tools.chat_mdl); a nil model means no model is available and the call skips,
// mirroring Python's no-op when tools.chat_mdl is absent. Never raises: on any
// chat/parse failure both results are empty, matching Python's except-return of
// an empty verdict.
func AskStructure(ctx context.Context, model SessionModel, topic, noun, label string, entities, relations []map[string]any) (string, []string) {
	if model == nil {
		_LOG.Printf("[%s] structure QA skipped (no chat model)", label)
		return "", nil
	}
	system := strings.ReplaceAll(navSystemPrompt, "{noun}", "the "+noun)
	rendered := RenderStructure(entities, relations)
	user := fmt.Sprintf("Question:\n%s\n\n%s:\n%s\n\nOutput JSON:", topic, capitalizeWord(noun), rendered)

	// Mirror Python _ask_structure:
	//   message_fit_in(form_message(system, user), tools.chat_mdl.max_length)
	// tools.chat_mdl.max_length is exposed via ContextLengthModel; when absent,
	// the 8192 default (chat EffectiveContextLength) applies, matching Python's
	// LLM.max_length defaulting when the model config omits max_tokens.
	budget := 0
	if cl, ok := model.(ContextLengthModel); ok {
		budget = cl.ContextLength()
	}
	if budget <= 0 {
		budget = chat.EffectiveContextLength(0)
	}
	fitted, fitErr := chat.FitMessages(system, []schema.Message{
		*schema.UserMessage(user),
	}, budget)
	if fitErr != "" {
		_LOG.Printf("[%s] prompt fitting failed: %s", label, fitErr)
		return "", nil
	}
	// FitMessages may prepend/trim a system message; re-extract it so the model
	// call is exactly [system, user...] as Python sends msg[0] then msg[1:].
	if len(fitted) > 0 && fitted[0].Role == schema.System {
		system = fitted[0].Content
		fitted = fitted[1:]
	}
	msgs := make([]schema.Message, 0, 1+len(fitted))
	msgs = append(msgs, *schema.SystemMessage(system))
	msgs = append(msgs, fitted...)
	// Python hardcodes {"temperature": 0.2} for this node (structure_qa.py:79)
	// — a stable verdict on a mechanical render, so it must not sample hot. The
	// value is a pinned constant: every production carrier implements
	// TemperatureModel (compile-time assertion on InvokerSessionModel), so the
	// temperature is always sent; a carrier without per-call temperature
	// support falls back to its own default (Go-only provider limitation).
	var resp *ModelReply
	var err error
	if tm, ok := model.(TemperatureModel); ok {
		resp, err = tm.CompleteWithTemperature(ctx, msgs, nil, structureQATemperature)
	} else {
		_LOG.Printf("[%s] model %T cannot carry per-call temperature; using its default (Python would send %v)", label, model, structureQATemperature)
		resp, err = model.Complete(ctx, msgs, nil)
	}
	if err != nil {
		_LOG.Printf("[%s] could not read the outline with the model: %v", label, err)
		return "", nil
	}

	var verdict structureNavVerdict
	if err := UnmarshalModelJSON(resp.Content, &verdict); err != nil {
		_LOG.Printf("[%s] could not parse the outline verdict: %v", label, err)
		return "", nil
	}

	answer := ""
	if verdict.IsSufficient {
		answer = strings.TrimSpace(verdict.Answer)
	}
	relevant := make([]string, 0, len(verdict.RelevantEntities))
	for _, n := range verdict.RelevantEntities {
		if s := strings.TrimSpace(n); s != "" {
			relevant = append(relevant, s)
		}
	}
	outcome := "does not fully answer"
	if verdict.IsSufficient {
		outcome = "answers"
	}
	_LOG.Printf("[%s] the %s %s the question; %d relevant entity(ies)", label, noun, outcome, len(relevant))
	return answer, relevant
}

// strAny reads a map field as its plain string form.
func strAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

// capitalizeWord mirrors Python str.capitalize(): uppercase the first rune and
// lowercase every remaining rune. Python lowercases the rest, so e.g.
// "hELLo".capitalize() -> "Hello"; the previous Go version left the tail
// untouched ("HELLO"), which diverged.
func capitalizeWord(s string) string {
	if s == "" {
		return s
	}
	rs := []rune(s)
	rs[0] = unicode.ToUpper(rs[0])
	for i := 1; i < len(rs); i++ {
		rs[i] = unicode.ToLower(rs[i])
	}
	return string(rs)
}
