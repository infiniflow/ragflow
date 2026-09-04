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

	"github.com/cloudwego/eino/schema"

	"gorm.io/gorm"
	"ragflow/internal/agent/chat"
)

// FINAL_ANSWER_SYSTEM mirrors Python report_prompt.FINAL_ANSWER_SYSTEM.
const finalAnswerSystem = `You are a smart agent. Answer the user's question using ONLY the evidence provided below. Do not invent facts: if the evidence cannot support a claim, say so plainly instead of guessing.

# Citation rules
{cite_rules}

# Language
Answer in the SAME language as the question. Translate retrieved evidence into that language as part of composing the answer; only verbatim quoted snippets may stay in their source language.

# Fallback
If the evidence does not answer the question, reply with a clear statement that you don't have enough information based on the available sources (in the user's language).
`

const partialAnswerPreamble = "Note: the following answer is based on partial information and may be incomplete."

const defaultCiteRules = "Cite passages with their source documents where available."

const (
	abstainMessage     = "I cannot answer this question based on the available information."
	emptyResultMessage = "I don't have enough information based on the available sources."
)

// AnswerResult mirrors the formalize_answer node output.
type AnswerResult struct {
	FinalAnswer string
	Abstained   bool
	Empty       bool
}

// FormalizeAnswer generates the final answer from the gathered kbinfos. Mirrors
// Python's formalize_answer node: abstain/empty short-circuits, otherwise builds
// system+user and calls the chat invoker.
//
// forceLLM forces the direct-LLM path even when there is no evidence: used by the
// FALLBACK_LLM verdict (FallbackToDirectLLM), where the mode is unanswerable but
// we still want the model to attempt an answer or plainly state it cannot — not
// a canned "no evidence" string. Mirrors Python, which only short-circuits on
// empty evidence when an explicit empty_response is configured.
// systemPrompt, when non-empty, is prepended to the final-answer system prompt
// so the agentic graph honors a configured dialog system prompt (mirrors Python
// RAGTools.system_prompt + formalize_answer).
func FormalizeAnswer(ctx context.Context, db *gorm.DB, question string, kb *Kbinfos, partial, abstain, empty bool, caveat string, forceLLM bool, systemPrompt string) AnswerResult {
	if abstain {
		return AnswerResult{FinalAnswer: abstainMessage, Abstained: true}
	}
	if !forceLLM && (empty || kb == nil || len(kb.Chunks) == 0) {
		return AnswerResult{FinalAnswer: emptyResultMessage, Empty: true}
	}

	var b strings.Builder
	b.WriteString("Question:\n" + question + "\n")
	// Research summary (merged claim reports from the high/ultra loop) precedes
	// the raw evidence, mirroring Python formalize_answer.
	if kb.PreSummary != "" {
		b.WriteString("\nResearch Summary:\n" + kb.PreSummary + "\n")
	}
	if partial {
		preamble := partialAnswerPreamble
		if caveat != "" {
			preamble = partialAnswerPreamble + " " + caveat
		}
		b.WriteString(preamble + "\n")
	}
	b.WriteString("\nEvidence:\n")
	for i, c := range kb.Chunks {
		text := chunkText(c)
		doc := chunkDoc(c)
		if text == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("[%d] %s", i+1, text))
		if doc != "" {
			b.WriteString(fmt.Sprintf(" (source: %s)", doc))
		}
		b.WriteString("\n")
	}

	system := strings.ReplaceAll(finalAnswerSystem, "{cite_rules}", defaultCiteRules)
	if strings.TrimSpace(systemPrompt) != "" {
		system = strings.TrimSpace(systemPrompt) + "\n\n" + system
	}
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return AnswerResult{FinalAnswer: "I'm sorry, the chat invoker is not configured."}
	}
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: system},
			{Role: schema.User, Content: b.String()},
		},
	})
	if err != nil {
		return AnswerResult{FinalAnswer: "I'm sorry, I encountered an error while composing the answer."}
	}
	return AnswerResult{FinalAnswer: resp.Content}
}

func chunkText(c map[string]interface{}) string {
	if t, ok := c["content_with_weight"].(string); ok && t != "" {
		return t
	}
	if t, ok := c["content"].(string); ok {
		return t
	}
	// "text" is the fallback used by the AutoRater evidence renderer (Python
	// _evidence_md: content_with_weight or text).
	if t, ok := c["text"].(string); ok {
		return t
	}
	return ""
}

func chunkDoc(c map[string]interface{}) string {
	if d, ok := c["docnm_kwd"].(string); ok {
		return d
	}
	if d, ok := c["doc_id"].(string); ok {
		return d
	}
	return ""
}
