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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// navNamingTemperature is the decoding temperature Python pins for both cluster
// prompts (_knowledge_compile_gen_conf(chat_mdl, {"temperature": 0.1})).
const navNamingTemperature = 0.1

// navNamingMaxRetries mirrors Python gen_json's max_retry=2 for the JSON naming
// prompt: each retry re-issues the call with the parse error appended, so a
// formatting hiccup costs two calls, not the compiler's default five.
const navNamingMaxRetries = 2

// NavMergeLLM is the production implementation of the nav.NavMergeLLM seam: it
// names dataset-nav clusters and fuses cluster descriptions with the tenant's
// chat model, mirroring Python dataset_nav._llm_create_summary / _llm_merge.
//
// It sits next to NavEmbedder in the service package, not with the seam it
// implements, because it needs ModelProviderService and no inner package may
// import service: service already imports nlp (chat_pipeline, deep_researcher,
// memory) and nav (dataset_artifact_service). The nav package is deliberately a
// dependency-light leaf — the agent tool layer and the nlp implementation both
// depend on it — so only the seam lives there and the model-backed adapters stay
// out here. Moving this type into nav would need the model access reduced to an
// injected interface first.
type NavMergeLLM struct {
	modelSvc *ModelProviderService
	// llmID is the configured chat model ref (a composite
	// "model@instance@provider" name or a tenant model id). Empty resolves the
	// tenant's default chat model, matching the dataset-level deduper's fallback.
	llmID string
}

// NewNavMergeLLM builds the production nav cluster namer/merger.
func NewNavMergeLLM(modelSvc *ModelProviderService, llmID string) *NavMergeLLM {
	return &NavMergeLLM{modelSvc: modelSvc, llmID: llmID}
}

// navNamingPrompt is the cluster-naming prompt (Python _llm_create_summary) plus
// a language constraint. The prompt is written in English, so the model answered
// in English even for a Chinese dataset, and the cluster name it produced (with
// the deterministic hash suffix appended) is what the nav tree displays — an
// English label over Chinese documents. The excerpts' own language is the one
// the tree's readers use, so it wins.
func navNamingPrompt(text string) string {
	return "Given the document excerpts below, produce a short human-readable topic " +
		"name and a concise description of their common topic.\n\n" +
		text + "\n\n" +
		"Write both the name and the description in the same language as the " +
		"excerpts (do not translate them into another language).\n\n" +
		`Return ONLY JSON: {"name": "<2-6 word topic title>", "summary": "<1-3 sentence description>"}`
}

// navMergePrompt fuses an existing cluster description with a new document
// summary (Python _llm_merge). Same language rule, anchored on the NEW text: a
// cluster whose description was written in another language converges back to
// the corpus language as documents are merged in, instead of drifting.
func navMergePrompt(existing, added string) string {
	return "Merge the following two descriptions of the same topic into " +
		"a single concise summary (1-3 sentences):\n\n" +
		"Existing: " + existing + "\n\n" +
		"New: " + added + "\n\n" +
		"Write the merged summary in the same language as the New description " +
		"(do not translate it).\n\n" +
		"Return ONLY the merged text, no commentary."
}

// CreateSummary asks the model for a short topic name plus a 1-3 sentence
// description of the supplied document text (Python _llm_create_summary). An
// empty name or an error makes the caller fall back to the deterministic
// "<summary first line> <hash>" name.
func (l *NavMergeLLM) CreateSummary(ctx context.Context, tenantID, text string) (name, summary string, err error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", nil
	}
	llmID, err := l.chatModelID(ctx, tenantID)
	if err != nil {
		return "", "", err
	}
	temp := navNamingTemperature
	reply, err := kccommon.GenJSON(ctx, navChatInvoker{modelSvc: l.modelSvc, tenantID: tenantID, llmID: llmID},
		kccommon.ChatRequest{UserPrompt: navNamingPrompt(text), Temperature: &temp}, navNamingMaxRetries)
	if err != nil {
		return "", "", err
	}
	name, summary = navSummaryFromReply(reply, text)
	return name, summary, nil
}

// Merge fuses the existing cluster description with a new document summary into
// one concise summary (Python _llm_merge(chat_mdl, cluster_desc, doc_summary)).
// texts is [existing description, new document summary]; the existing text is
// returned unchanged when there is nothing to merge or the model reply is
// unusable, so the caller keeps the stored description.
func (l *NavMergeLLM) Merge(ctx context.Context, tenantID string, texts []string) (string, error) {
	existing := ""
	if len(texts) > 0 {
		existing = strings.TrimSpace(texts[0])
	}
	added := ""
	if len(texts) > 1 {
		added = strings.TrimSpace(texts[1])
	}
	if added == "" {
		return existing, nil
	}
	llmID, err := l.chatModelID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	temp := navNamingTemperature
	reply, err := navChatText(ctx, l.modelSvc, tenantID, llmID, "", navMergePrompt(existing, added), &temp)
	if err != nil {
		return "", err
	}
	return navMergedFromReply(reply, existing), nil
}

// chatModelID resolves the chat model ref to call: the configured one, else the
// tenant's default chat model.
func (l *NavMergeLLM) chatModelID(ctx context.Context, tenantID string) (string, error) {
	if l.modelSvc == nil {
		return "", fmt.Errorf("datasetnav: model provider service not initialized")
	}
	if ref := strings.TrimSpace(l.llmID); ref != "" {
		return ref, nil
	}
	target, err := l.modelSvc.modelSolver().ResolveDefaultModelConfig(ctx, tenantID, entity.ModelTypeChat)
	if err != nil {
		return "", fmt.Errorf("datasetnav: resolve default chat model for tenant %s: %w", tenantID, err)
	}
	if target == nil || strings.TrimSpace(target.ModelID) == "" {
		return "", fmt.Errorf("datasetnav: no default chat model for tenant %s", tenantID)
	}
	return target.ModelID, nil
}

// navSummaryFromReply mirrors Python _llm_create_summary's response handling:
// name from "name", summary from "summary" then "result", the input text as the
// last resort. An empty name is returned as-is so the caller falls back to its
// deterministic name.
func navSummaryFromReply(reply map[string]any, fallbackSummary string) (name, summary string) {
	summary = navReplyString(reply["summary"])
	if summary == "" {
		summary = navReplyString(reply["result"])
	}
	if summary == "" {
		summary = strings.TrimSpace(fallbackSummary)
	}
	return navReplyString(reply["name"]), summary
}

// navMergedFromReply mirrors Python _llm_merge's reply handling, with one
// deliberate deviation: a plain-text reply IS used. Python feeds the reply
// through gen_json, whose json_repair parse of ordinary prose yields an empty
// string, so its `isinstance(resp, str) and resp.strip()` branch never fires and
// the merge silently keeps the old description — even though the prompt asks for
// "only the merged text". Keeping the reply body is what the prompt intends.
func navMergedFromReply(reply, existing string) string {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return existing
	}
	if repaired, err := kccommon.RepairJSONText(reply); err == nil {
		var m map[string]any
		if json.Unmarshal([]byte(repaired), &m) == nil {
			if merged := navReplyString(m["merged"]); merged != "" {
				return merged
			}
			if merged := navReplyString(m["result"]); merged != "" {
				return merged
			}
			return existing
		}
	}
	return reply
}

// navReplyString reads a string field out of an LLM JSON reply. Non-string
// values are ignored: the prompts only ever ask for strings, and stringifying a
// nested object would leak a debug repr into a stored description.
func navReplyString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// navChatInvoker adapts the model provider service to the knowledge-compiler
// ChatInvoker seam, so the JSON naming prompt reuses kccommon.GenJSON's
// extraction/repair/retry handling (the Go counterpart of Python gen_json).
type navChatInvoker struct {
	modelSvc *ModelProviderService
	tenantID string
	llmID    string
}

func (c navChatInvoker) Chat(ctx context.Context, req kccommon.ChatRequest) (*kccommon.ChatResponse, error) {
	content, err := navChatText(ctx, c.modelSvc, c.tenantID, c.llmID, req.SystemPrompt, req.UserPrompt, req.Temperature)
	if err != nil {
		return nil, err
	}
	return &kccommon.ChatResponse{Content: content}, nil
}

// navChatText dispatches one non-streaming chat call. system may be empty (both
// nav prompts are single-turn, matching Python's gen_json("", prompt, ...)).
func navChatText(ctx context.Context, modelSvc *ModelProviderService, tenantID, llmID, system, user string, temperature *float64) (string, error) {
	if modelSvc == nil {
		return "", fmt.Errorf("datasetnav: model provider service not initialized")
	}
	if strings.TrimSpace(llmID) == "" {
		return "", fmt.Errorf("datasetnav: chat model is not resolved")
	}
	config := &modelModule.ChatConfig{}
	if temperature != nil {
		config.Temperature = temperature
	}
	msgs := []modelModule.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	resp, err := modelSvc.Chat(ctx, tenantID, llmID, msgs, config)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Answer == nil {
		return "", nil
	}
	return strings.TrimSpace(*resp.Answer), nil
}
