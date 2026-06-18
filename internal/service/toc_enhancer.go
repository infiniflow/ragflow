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
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"

	"github.com/kaptinlin/jsonrepair"
	"go.uber.org/zap"
)

// flexInt is an int that can unmarshal from either a JSON number or a JSON string.
// This handles the mismatch between DB-stored TOC entries (level as string "1")
// and LLM-emitted scores (level as number 1).
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*f = flexInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("flexInt: invalid string %q: %w", s, err)
		}
		*f = flexInt(i)
		return nil
	}
	return fmt.Errorf("flexInt: cannot unmarshal %s", string(data))
}

func (f flexInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(f))
}

// tocEntry holds a single entry from a document's TOC chunk.
// Note: level is stored as a string in JSON (e.g. "1"), so we use flexInt.
type tocEntry struct {
	Level flexInt  `json:"level"`
	Title string   `json:"title"`
	IDs   []string `json:"ids,omitempty"`
}

// tocRelevanceScore is the LLM-emitted score for a single TOC entry.
type tocRelevanceScore struct {
	Level int     `json:"level"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

const tocRelevanceSystemPrompt = `You are an expert logical reasoning assistant specializing in hierarchical Table of Contents (TOC) relevance evaluation.

## GOAL
You will receive:
1. A JSON list of TOC items, each with fields:
   ` + "```" + `json
   {
     "level": <integer>,   // e.g., 1, 2, 3
     "title": <string>     // section title
}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}
   ` + "```" + `
2. A user query (natural language question).

You must assign a **relevance score** (integer) to every TOC entry, based on how related its ` + "`" + `title` + "`" + ` is to the ` + "`" + `query` + "`" + `.

---

## RULES

### Scoring System
- 5 → highly relevant (directly answers or matches the query intent)
- 3 → somewhat related (same topic or partially overlaps)
- 1 → weakly related (vague or tangential)
- 0 → no clear relation
- -1 → explicitly irrelevant or contradictory

### Hierarchy Traversal
- The TOC is hierarchical: smaller ` + "`" + `level` + "`" + ` = higher layer (e.g., level 1 is top-level, level 2 is a subsection).
- You must traverse in **hierarchical order** — interpret the structure based on levels (1 > 2 > 3).
- If a high-level item (level 1) is strongly related (score 5), its child items (level 2, 3) are likely relevant too.
- If a high-level item is unrelated (-1 or 0), its deeper children are usually less relevant unless the titles clearly match the query.
- Lower (deeper) levels provide more specific content; prefer assigning higher scores if they directly match the query.

### Output Format
Return a **JSON array**, preserving the input order but adding a new key ` + "`" + `"score"` + "`" + `:

` + "```" + `json
[
  {"level": 1, "title": "Introduction", "score": 0},
  {"level": 2, "title": "Definition of Sustainability", "score": 5}
]
` + "```" + `

### Constraints
- Output **only the JSON array** — no explanations or reasoning text.

### EXAMPLES

#### Example 1
Input TOC:
[
  {"level": 1, "title": "Machine Learning Overview"},
  {"level": 2, "title": "Supervised Learning"},
  {"level": 2, "title": "Unsupervised Learning"},
  {"level": 3, "title": "Applications of Deep Learning"}
]

Query:
"How is deep learning used in image classification?"

Output:
[
  {"level": 1, "title": "Machine Learning Overview", "score": 3},
  {"level": 2, "title": "Supervised Learning", "score": 3},
  {"level": 2, "title": "Unsupervised Learning", "score": 0},
  {"level": 3, "title": "Applications of Deep Learning", "score": 5}
]

---

#### Example 2
Input TOC:
[
  {"level": 1, "title": "Marketing Basics"},
  {"level": 2, "title": "Consumer Behavior"},
  {"level": 2, "title": "Digital Marketing"},
  {"level": 3, "title": "Social Media Campaigns"},
  {"level": 3, "title": "SEO Optimization"}
]

Query:
"What are the best online marketing methods?"

Output:
[
  {"level": 1, "title": "Marketing Basics", "score": 3},
  {"level": 2, "title": "Consumer Behavior", "score": 1},
  {"level": 2, "title": "Digital Marketing", "score": 5},
  {"level": 3, "title": "Social Media Campaigns", "score": 5},
  {"level": 3, "title": "SEO Optimization", "score": 5}
]

---

#### Example 3
Input TOC:
[
  {"level": 1, "title": "Physics Overview"},
  {"level": 2, "title": "Classical Mechanics"},
  {"level": 3, "title": "Newton's Laws"},
  {"level": 2, "title": "Thermodynamics"},
  {"level": 3, "title": "Entropy and Heat Transfer"}
]

Query:
"What is entropy?"

Output:
[
  {"level": 1, "title": "Physics Overview", "score": 3},
  {"level": 2, "title": "Classical Mechanics", "score": 0},
  {"level": 3, "title": "Newton's Laws", "score": -1},
  {"level": 2, "title": "Thermodynamics", "score": 5},
  {"level": 3, "title": "Entropy and Heat Transfer", "score": 5}
]
`

const tocRelevanceUserTemplate = `You will now receive:
1. A JSON list of TOC items (each with ` + "`" + `level` + "`" + ` and ` + "`" + `title` + "`" + `)
2. A user query string.

Traverse the TOC hierarchically based on level numbers and assign scores (5,3,1,0,-1) according to the rules in the system prompt.
Output **only** the JSON array with the added ` + "`" + `"score"` + "`" + ` field.

---

**Input TOC:**
%s

**Query:**
%s
`

// TOCEnhancer picks the top document, fetches its TOC, scores entries via LLM,
// then merges matching chunks into kbinfos["chunks"].
type TOCEnhancer struct {
	docEngine engine.DocEngine
	chatModel *modelModule.ChatModel
	tenantIDs []string
	kbIDs     []string
	question  string
	topN      int
}

// NewTOCEnhancer constructs a TOCEnhancer.
func NewTOCEnhancer(
	docEngine engine.DocEngine,
	chatModel *modelModule.ChatModel,
	tenantIDs []string,
	kbIDs []string,
	question string,
	topN int,
) *TOCEnhancer {
	return &TOCEnhancer{
		docEngine: docEngine,
		chatModel: chatModel,
		tenantIDs: tenantIDs,
		kbIDs:     kbIDs,
		question:  question,
		topN:      topN,
	}
}

// Enhance mutates kbinfos["chunks"] by appending/boosting TOC-relevant chunks.
func (e *TOCEnhancer) Enhance(ctx context.Context, kbinfos map[string]interface{}) (int, error) {
	if e == nil || e.chatModel == nil {
		return 0, nil
	}
	if kbinfos == nil {
		return 0, nil
	}
	if e.docEngine == nil {
		e.docEngine = engine.Get()
	}
	if e.docEngine == nil {
		return 0, nil
	}
	chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{})
	if !ok || len(chunksRaw) == 0 {
		return 0, nil
	}

	common.Debug("TOC enhancer: started",
		zap.Int("chunk_count", len(chunksRaw)),
		zap.String("question", e.question))

	topDocID, docID2KBID := topDocFromChunks(chunksRaw)
	if topDocID == "" {
		return 0, nil
	}

	filter := map[string]interface{}{
		"doc_id":  []string{topDocID},
		"toc_kwd": "toc",
	}
	indexNames := make([]string, 0, len(e.tenantIDs))
	for _, tid := range e.tenantIDs {
		indexNames = append(indexNames, indexName(tid))
	}
	tocResp, err := e.docEngine.Search(ctx, &types.SearchRequest{
		IndexNames:   indexNames,
		KbIDs:        e.kbIDs,
		Filter:       filter,
		SelectFields: []string{"content_with_weight"},
		Offset:       0,
		Limit:        128,
	})
	if err != nil || tocResp == nil || len(tocResp.Chunks) == 0 {
		common.Debug("TOC enhancer: no TOC chunks found for top doc",
			zap.String("doc_id", topDocID))
		return 0, nil
	}

	entries := parseTOCEntries(tocResp.Chunks)
	if len(entries) == 0 {
		common.Debug("TOC enhancer: TOC content did not parse to entries",
			zap.String("doc_id", topDocID))
		return 0, nil
	}

	scores, err := e.scoreEntries(ctx, entries, e.topN*2)
	if err != nil {
		common.Warn("TOC enhancer: LLM scoring failed",
			zap.Error(err), zap.String("doc_id", topDocID))
		return 0, nil
	}
	if len(scores) == 0 {
		return 0, nil
	}

	id2idx := map[string]int{}
	for i, cm := range chunksRaw {
		if cid, ok := cm["chunk_id"].(string); ok && cid != "" {
			id2idx[cid] = i
		}
	}
	added := 0
	kbID := docID2KBID[topDocID]
	for _, sc := range scores {
		cid := sc.Title
		if idx, exists := id2idx[cid]; exists {
			boostSimilarity(chunksRaw[idx], sc.Score)
		} else {
			fresh, fetchErr := e.fetchChunk(ctx, cid, topDocID, kbID)
			if fetchErr != nil || fresh == nil {
				continue
			}
			d := map[string]interface{}{
				"chunk_id":            cid,
				"content_ltks":        getString(fresh, "content_ltks"),
				"content_with_weight": getString(fresh, "content_with_weight"),
				"doc_id":              topDocID,
				"docnm_kwd":           getStringDef(fresh, "docnm_kwd", ""),
				"kb_id":               getStringDef(fresh, "kb_id", kbID),
				"important_kwd":       getSlice(fresh, "important_kwd"),
				"image_id":            getStringDef(fresh, "img_id", getStringDef(fresh, "image_id", "")),
				"similarity":          sc.Score,
				"vector_similarity":   sc.Score,
				"term_similarity":     sc.Score,
				"vector":              []float64{},
				"positions":           getSlice(fresh, "position_int"),
				"doc_type_kwd":        getStringDef(fresh, "doc_type_kwd", ""),
			}
			for k, v := range fresh {
				if len(k) >= 4 && k[len(k)-4:] == "_vec" {
					if vec := toFloat64Slice(v); vec != nil {
						d["vector"] = vec
						break
					}
				}
			}
			chunksRaw = append(chunksRaw, d)
			id2idx[cid] = len(chunksRaw) - 1
			added++
		}
	}

	kbinfos["chunks"] = sortAndTrimChunks(chunksRaw, e.topN)
	common.Debug("TOC enhancer: finished",
		zap.Int("added_chunks", added),
		zap.Int("total_chunks", len(chunksRaw)),
		zap.String("doc_id", topDocID))
	return added, nil
}

// topDocFromChunks picks the doc_id with the highest accumulated similarity.
func topDocFromChunks(chunks []map[string]interface{}) (string, map[string]string) {
	ranks := map[string]float64{}
	docID2KBID := map[string]string{}
	for _, cm := range chunks {
		docID, _ := cm["doc_id"].(string)
		kbID, _ := cm["kb_id"].(string)
		sim, _ := cm["similarity"].(float64)
		if docID == "" {
			continue
		}
		ranks[docID] += sim
		if _, seen := docID2KBID[docID]; !seen && kbID != "" {
			docID2KBID[docID] = kbID
		}
	}
	if len(ranks) == 0 {
		return "", nil
	}
	type kv struct {
		k string
		v float64
	}
	pairs := make([]kv, 0, len(ranks))
	for k, v := range ranks {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	return pairs[0].k, docID2KBID
}

// parseTOCEntries flattens TOC entries across all TOC chunks.
func parseTOCEntries(chunks []map[string]interface{}) []tocEntry {
	common.Debug("TOC enhancer: parsing TOC entries",
		zap.Int("chunk_count", len(chunks)))
	var out []tocEntry
	for _, ck := range chunks {
		cww, _ := ck["content_with_weight"].(string)
		if cww == "" {
			continue
		}
		var arr []tocEntry
		if err := json.Unmarshal([]byte(cww), &arr); err == nil {
			out = append(out, arr...)
			continue
		}
		var single tocEntry
		if err := json.Unmarshal([]byte(cww), &single); err == nil && single.Title != "" {
			out = append(out, single)
			continue
		}
		// Debug: log raw content that failed to parse
		preview := cww
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		chunkID, _ := ck["id"].(string)
		docID, _ := ck["doc_id"].(string)
		common.Debug("TOC enhancer: chunk content not valid TOC JSON",
			zap.String("chunk_id", chunkID),
			zap.String("doc_id", docID),
			zap.String("content_preview", preview))
	}
	return out
}

// scoreEntries calls the LLM to score TOC entries and returns (chunkID, normalizedScore) pairs.
func (e *TOCEnhancer) scoreEntries(ctx context.Context, entries []tocEntry, limit int) ([]tocRelevanceScore, error) {
	if e.chatModel == nil || e.chatModel.ModelDriver == nil || len(entries) == 0 {
		return nil, nil
	}

	type tocLLMInput struct {
		Level int    `json:"level"`
		Title string `json:"title"`
	}
	lines := make([]string, len(entries))
	for i, ent := range entries {
		b, _ := json.Marshal(tocLLMInput{Level: int(ent.Level), Title: ent.Title})
		lines[i] = string(b)
	}
	tocStr := fmt.Sprintf("[\n%s\n]\n", strings.Join(lines, "\n"))

	userPrompt := fmt.Sprintf(tocRelevanceUserTemplate, tocStr, e.question)

	tempZero := 0.0
	topP := 0.9
	cfg := &modelModule.ChatConfig{
		Temperature: &tempZero,
		TopP:        &topP,
	}

	var scores []tocRelevanceScore
	maxRetry := 2
	var lastAns, lastErr string
	for attempt := 0; attempt < maxRetry; attempt++ {
		currentUser := userPrompt
		if attempt > 0 && lastAns != "" && lastErr != "" {
			currentUser += fmt.Sprintf(
				"\nGenerated JSON is as following:\n%s\nBut exception while loading:\n%s\nPlease reconsider and correct it.",
				lastAns, lastErr,
			)
		}
		msgs := []modelModule.Message{
			{Role: "system", Content: tocRelevanceSystemPrompt},
			{Role: "user", Content: currentUser},
		}
		modelName := ""
		if e.chatModel.ModelName != nil {
			modelName = *e.chatModel.ModelName
		}
		resp, err := e.chatModel.ModelDriver.ChatWithMessages(
			modelName, msgs, e.chatModel.APIConfig, cfg,
		)
		if err != nil {
			return nil, err
		}
		if resp == nil || resp.Answer == nil {
			return nil, fmt.Errorf("toc scoring: empty response")
		}

		raw := cleanLLMResponse(*resp.Answer)
		lastAns = raw

		repaired, rerr := jsonrepair.Repair(raw)
		if rerr != nil {
			repaired = raw
		}
		if err := json.Unmarshal([]byte(repaired), &scores); err != nil {
			lastErr = err.Error()
			common.Warn("TOC enhancer: JSON parse failed, retrying",
				zap.Error(err), zap.Int("attempt", attempt))
			continue
		}
		break
	}
	if len(scores) == 0 && lastErr != "" {
		return nil, fmt.Errorf("toc scoring: parse failed after retries: %s", lastErr)
	}

	id2score := make(map[string][]float64)
	for i := 0; i < len(scores) && i < len(entries); i++ {
		sc := scores[i]
		if sc.Score < 1 {
			continue
		}
		norm := sc.Score / 5.0
		for _, cid := range entries[i].IDs {
			id2score[cid] = append(id2score[cid], norm)
		}
	}

	result := make([]tocRelevanceScore, 0, len(id2score))
	for cid, vals := range id2score {
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		avg := sum / float64(len(vals))
		if avg >= 0.3 {
			result = append(result, tocRelevanceScore{
				Title: cid,
				Score: avg,
			})
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// fetchChunk loads a single chunk by chunk_id from the engine.
func (e *TOCEnhancer) fetchChunk(ctx context.Context, chunkID, docID, kbID string) (map[string]interface{}, error) {
	filter := map[string]interface{}{
		"doc_id":   []string{docID},
		"chunk_id": []string{chunkID},
	}
	indexNames := make([]string, 0, len(e.tenantIDs))
	for _, tid := range e.tenantIDs {
		indexNames = append(indexNames, indexName(tid))
	}
	resp, err := e.docEngine.Search(ctx, &types.SearchRequest{
		IndexNames:   indexNames,
		KbIDs:        []string{kbID},
		Filter:       filter,
		SelectFields: []string{"content_with_weight", "content_ltks", "doc_id", "docnm_kwd", "kb_id", "important_kwd", "image_id", "positions", "doc_type_kwd", "vector", "q_1024_vec"},
		Offset:       0,
		Limit:        1,
	})
	if err != nil || resp == nil || len(resp.Chunks) == 0 {
		return nil, fmt.Errorf("toc enhancer: fetch chunk %s: not found", chunkID)
	}
	return resp.Chunks[0], nil
}

// indexName returns the search index name for a tenant.
func indexName(tenantID string) string {
	return "ragflow_" + tenantID
}

func boostSimilarity(cm map[string]interface{}, delta float64) {
	cm["similarity"] = getFloat(cm, "similarity") + delta
}

func getStringDef(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

func getSlice(m map[string]interface{}, key string) []interface{} {
	if v, ok := m[key].([]interface{}); ok {
		return v
	}
	return nil
}

// sortAndTrimChunks sorts chunks by similarity descending and trims to top-N.
func sortAndTrimChunks(chunks []map[string]interface{}, topN int) []map[string]interface{} {
	sort.SliceStable(chunks, func(i, j int) bool {
		return getFloat(chunks[i], "similarity") > getFloat(chunks[j], "similarity")
	})
	if topN > 0 && topN < len(chunks) {
		chunks = chunks[:topN]
	}
	return chunks
}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}
