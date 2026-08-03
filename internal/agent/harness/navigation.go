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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// Structure navigation mirrors Python navigation.py (ontology_navigate /
// mindmap_navigate). These live in the harness package (not agent/tool) because
// they need the chat invoker, and agent/tool → agent/component would form an
// import cycle.
const (
	toolOntologyNavigate = "ontology_navigate"
	toolMindmapNavigate  = "mindmap_navigate"
)

var catalogKinds = map[string]bool{"tree": true, "timeline": true, "raptor": true, "page_index": true, "pageindex": true}
var mindmapKinds = map[string]bool{"mindmap": true, "mind_map": true}

const navSystemPrompt = `You are given the {noun} of one or more documents — an outline of entities and their relations — and a question.

Decide whether that outline alone already answers the question.

Rules:
1. Answer ONLY from the outline below. Do not invent facts.
2. Set "is_sufficient" to true only when the outline genuinely answers the question; otherwise false with an empty answer.
3. Always fill "relevant_entities" with the exact ` + "`name`" + ` values of the entities most related to the question (up to 10), even when the outline is not sufficient — they are used to pull the underlying source text.

Output ONLY JSON, no prose, no code fences:
{"is_sufficient": true/false, "answer": "<answer, or empty>", "relevant_entities": ["<entity name>", ...]}`

const (
	maxStructureEntities  = 300
	maxStructureRelations = 300
	maxEvidenceChunks     = 24
)

type structureNavArgs struct {
	Topic    string   `json:"topic"`
	Keywords string   `json:"keywords,omitempty"`
	DocScope []string `json:"doc_scope,omitempty"`
}

type structureEntity struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Description    string   `json:"description"`
	SourceChunkIDs []string `json:"source_chunk_ids"`
	DocID          string   `json:"-"`
}

type structureNavVerdict struct {
	IsSufficient     bool     `json:"is_sufficient"`
	Answer           string   `json:"answer"`
	RelevantEntities []string `json:"relevant_entities"`
}

// NavigateStructure implements ontology_navigate / mindmap_navigate. It reads
// the compiled structure (entities) of the in-scope documents, asks the chat
// model which entities answer the question, and pulls the source chunks behind
// the selected entities. Routing only — returns empty on any failure.
func NavigateStructure(ctx context.Context, tenantID string, kind string, args structureNavArgs) (string, error) {
	noun := "catalog"
	var kinds map[string]bool
	if kind == toolMindmapNavigate {
		kinds = mindmapKinds
		noun = "mindmap"
	} else {
		kinds = catalogKinds
	}

	query := strings.TrimSpace(args.Topic + " " + args.Keywords)
	if query == "" || len(args.DocScope) == 0 {
		return `{"chunks":[]}`, nil
	}

	var entities []structureEntity
	for _, docID := range args.DocScope {
		es := loadStructureEntities(ctx, tenantID, docID, kinds)
		for _, e := range es {
			if e.Name != "" {
				e.DocID = docID
				entities = append(entities, e)
			}
		}
	}
	if len(entities) == 0 {
		return `{"chunks":[]}`, nil
	}

	selected, err := askStructureSelect(ctx, query, noun, entities)
	if err != nil || len(selected) == 0 {
		return `{"chunks":[]}`, nil
	}

	idsByDoc := map[string][]string{}
	for _, e := range selected {
		idsByDoc[e.DocID] = append(idsByDoc[e.DocID], e.SourceChunkIDs...)
	}
	var chunks []map[string]interface{}
	for _, ids := range idsByDoc {
		chunks = append(chunks, loadChunksByIDs(ctx, tenantID, dedupStrings(ids))...)
		if len(chunks) >= maxEvidenceChunks {
			break
		}
	}
	b, _ := json.Marshal(map[string]interface{}{"chunks": chunks})
	return string(b), nil
}

func loadStructureEntities(ctx context.Context, tenantID, docID string, kinds map[string]bool) []structureEntity {
	de := engine.Get()
	if de == nil {
		return nil
	}
	idx := fmt.Sprintf("ragflow_%s", tenantID)
	req := &types.SearchRequest{
		IndexNames:   []string{idx},
		Filter:       map[string]interface{}{"doc_id": []string{docID}, "knowledge_graph_kwd": []string{"graph"}},
		SelectFields: []string{"content_with_weight", "compile_kwd", "compilation_template_kind_kwd"},
		Limit:        1000,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil
	}
	var out []structureEntity
	for _, row := range res.Chunks {
		kind := normalizeKind(row)
		if !kinds[kind] {
			continue
		}
		payload, _ := row["content_with_weight"].(string)
		var graph struct {
			Entities []structureEntity `json:"entities"`
		}
		if err := json.Unmarshal([]byte(payload), &graph); err != nil {
			continue
		}
		out = append(out, graph.Entities...)
	}
	return out
}

func normalizeKind(row map[string]interface{}) string {
	if ck, _ := row["compile_kwd"].(string); ck == "raptor_graph" {
		return "raptor"
	}
	kind, _ := row["compilation_template_kind_kwd"].(string)
	if kind == "" {
		kind, _ = row["compile_kwd"].(string)
	}
	kind = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(kind, "-", "_")))
	if kind == "pageindex" || kind == "page_index" || kind == "knowledge_graph" {
		return "timeline"
	}
	return kind
}

func askStructureSelect(ctx context.Context, query, noun string, entities []structureEntity) ([]structureEntity, error) {
	rendered := renderStructureEntities(entities)
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return nil, fmt.Errorf("dataset navigation: chat invoker not configured")
	}
	resp, err := inv.Invoke(ctx, nil, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: strings.ReplaceAll(navSystemPrompt, "{noun}", noun)},
			{Role: schema.User, Content: fmt.Sprintf("Question:\n%s\n\n%s:\n%s\n\nOutput JSON:", query, noun, rendered)},
		},
	})
	if err != nil {
		return nil, err
	}
	var v structureNavVerdict
	if err := unmarshalModelJSON(resp.Content, &v); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range v.RelevantEntities {
		want[n] = true
	}
	var out []structureEntity
	for _, e := range entities {
		if want[e.Name] {
			out = append(out, e)
		}
	}
	return out, nil
}

func renderStructureEntities(entities []structureEntity) string {
	var b strings.Builder
	b.WriteString("Entities:")
	for i, e := range entities {
		if i >= maxStructureEntities {
			break
		}
		b.WriteString("\n- " + e.Name + " (" + orStr(e.Type, "other") + ")")
		if d := strings.Join(strings.Fields(e.Description), " "); d != "" {
			b.WriteString(": " + d)
		}
	}
	return b.String()
}

func loadChunksByIDs(ctx context.Context, tenantID string, ids []string) []map[string]interface{} {
	if len(ids) == 0 {
		return nil
	}
	de := engine.Get()
	if de == nil {
		return nil
	}
	idx := fmt.Sprintf("ragflow_%s", tenantID)
	limit := maxEvidenceChunks
	if len(ids) < limit {
		limit = len(ids)
	}
	req := &types.SearchRequest{
		IndexNames:   []string{idx},
		Filter:       map[string]interface{}{"id": ids},
		SelectFields: []string{"content_with_weight", "docnm_kwd", "doc_id"},
		Limit:        limit,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	for _, row := range res.Chunks {
		out = append(out, map[string]interface{}{
			"chunk_id": row["id"], "content_with_weight": row["content_with_weight"],
			"docnm_kwd": row["docnm_kwd"], "doc_id": row["doc_id"],
		})
	}
	return out
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
