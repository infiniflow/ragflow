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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/kg"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"

	"github.com/google/uuid"
	"github.com/kaptinlin/jsonrepair"
	"go.uber.org/zap"
)

// Prompt templates

const sufficiencyCheckTemplate = `You are a information retrieval evaluation expert. Please assess whether the currently retrieved content is sufficient to answer the user's question.

User question:
%s

Retrieved content:
%s

Please determine whether these content are sufficient to answer the user's question.

Output format (JSON):
{
    "is_sufficient": true/false,
    "reasoning": "Your reasoning for the judgment",
    "missing_information": ["Missing information 1", "Missing information 2"]
}

Requirements:
1. If the retrieved content contains key information needed to answer the query, judge as sufficient (true).
2. If key information is missing, judge as insufficient (false), and list the missing information.
3. The reasoning should be concise and clear.
4. The missing_information should only be filled when insufficient, otherwise empty array.
`

const multiQueriesGenTemplate = `You are a query optimization expert. 
The user's original query failed to retrieve sufficient information; 
please generate multiple complementary improved questions and corresponding queries.

Original query:
%s

Original question:
%s

Currently, retrieved content:
%s

Missing information:
%s

Please generate 2-3 complementary queries to help find the missing information. These queries should:
1. Focus on different missing information points.
2. Use different expressions.
3. Avoid being identical to the original query.
4. Remain concise and clear.

Output format (JSON):
{
    "reasoning": "Explanation of query generation strategy",
    "questions": [
        {"question": "Improved question 1", "query": "Improved query 1"},
        {"question": "Improved question 2", "query": "Improved query 2"},
        {"question": "Improved question 3", "query": "Improved query 3"}
    ]
}

Requirements:
1. Questions array contains 1-3 questions and corresponding queries.
2. Each question length is between 5-200 characters.
3. Each query length is between 1-5 keywords.
4. Each query MUST be in the same language as the retrieved content in. 
5. DO NOT generate question and query that is similar to the original query. 
6. Reasoning explains the generation strategy.
`

// Types

// KBRetrieveFunc is the signature for knowledge base retrieval.
type KBRetrieveFunc func(ctx context.Context, question string) (*nlp.RetrievalResult, error)

// sufficiencyResult is the sufficiency_check JSON output.
type sufficiencyResult struct {
	IsSufficient       bool     `json:"is_sufficient"`
	Reasoning          string   `json:"reasoning"`
	MissingInformation []string `json:"missing_information"`
}

// queryPair is a {question, query} entry.
type queryPair struct {
	Question string `json:"question"`
	Query    string `json:"query"`
}

// multiQueriesResult is the multi_queries_gen JSON output.
type multiQueriesResult struct {
	Reasoning string      `json:"reasoning"`
	Questions []queryPair `json:"questions"`
}

// DeepResearcher implements recursive query-decomposition retrieval.
// Each level: retrieve → sufficiency check → if insufficient, generate
// sub-queries → recurse. Accumulates chunks into a shared chunkInfo map.
type DeepResearcher struct {
	ChatModel       *modelModule.ChatModel
	PromptConfig    map[string]interface{}
	KBRetrieve      KBRetrieveFunc
	InternetEnabled bool
	TavilyAPIKey    string

	// Fields needed for KG retrieval (mirrors async_chat.go usage).
	DocEngine engine.DocEngine
	KbIDs     []string
	TenantIDs []string
	EmbModel  *modelModule.EmbeddingModel

	maxDepth  int // default 3
	mu        sync.Mutex
	tavilyURL string
}

// NewDeepResearcher constructs a DeepResearcher.
func NewDeepResearcher(
	chatModel *modelModule.ChatModel,
	promptConfig map[string]interface{},
	kbRetrieve KBRetrieveFunc,
	internetEnabled bool,
	docEngine engine.DocEngine,
	kbIDs []string,
	tenantIDs []string,
	embModel *modelModule.EmbeddingModel,
) *DeepResearcher {
	return &DeepResearcher{
		ChatModel:       chatModel,
		PromptConfig:    promptConfig,
		KBRetrieve:      kbRetrieve,
		InternetEnabled: internetEnabled,
		TavilyAPIKey:    mapStringValue(promptConfig, "tavily_api_key"),
		DocEngine:       docEngine,
		KbIDs:           kbIDs,
		TenantIDs:       tenantIDs,
		EmbModel:        embModel,
		maxDepth:        3,
		tavilyURL:       "https://api.tavily.com/search",
	}
}

// Research runs the recursive tree search, accumulating chunks into chunkInfo.
func (dr *DeepResearcher) Research(
	ctx context.Context,
	chunkInfo map[string]interface{},
	question string,
	query string,
	callback func(string),
) error {
	if dr == nil || dr.ChatModel == nil {
		return fmt.Errorf("DeepResearcher: missing chat model")
	}

	if callback != nil {
		callback("<START_DEEP_RESEARCH>")
	}

	// Initialize chunkInfo if empty
	if _, ok := chunkInfo["chunks"]; !ok {
		chunkInfo["chunks"] = []interface{}{}
		chunkInfo["doc_aggs"] = []interface{}{}
		chunkInfo["total"] = 0
	}

	_, err := dr._research(ctx, chunkInfo, question, query, dr.maxDepth, callback)
	if err != nil {
		common.Warn("DeepResearcher: research failed", zap.Error(err))
	}

	if callback != nil {
		callback("<END_DEEP_RESEARCH>")
	}

	return err
}

// _research is the recursive depth-first worker.
func (dr *DeepResearcher) _research(
	ctx context.Context,
	chunkInfo map[string]interface{},
	question string,
	query string,
	depth int,
	callback func(string),
) (string, error) {
	if depth == 0 {
		return "", nil
	}

	if callback != nil {
		callback(fmt.Sprintf("Searching by `%s`...", query))
	}

	// 1. Retrieve information (KB + optional web)
	st := time.Now()
	kbinfos, err := dr._retrieve_information(ctx, query)
	if err != nil {
		return "", err
	}

	if callback != nil {
		n := len(chunksFromKBInfos(kbinfos))
		elapsed := time.Since(st).Milliseconds()
		callback(fmt.Sprintf("Retrieval %d results in %.1fms", n, float64(elapsed)))
	}

	// 2. Merge into chunkInfo (dedup by chunk_id)
	dr.mergeChunkInfo(chunkInfo, kbinfos)

	// 3. Trim content
	maxTokens := dr.ChatModelMaxTokens() / 2
	knowledges := kbPrompt(kbinfos, maxTokens, false)
	retContent := strings.Join(knowledges, "\n\n")

	// 4. Sufficiency check
	if callback != nil {
		callback("Checking the sufficiency for retrieved information.")
	}

	suff, err := dr.sufficiencyCheck(ctx, question, retContent)
	if err != nil {
		common.Warn("DeepResearcher: sufficiency check failed",
			zap.Error(err), zap.Int("depth", depth))
		// On error, treat as insufficient
		suff = &sufficiencyResult{IsSufficient: false}
	}

	if suff.IsSufficient {
		if callback != nil {
			callback(fmt.Sprintf("Yes, the retrieved information is sufficient for '%s'.", question))
		}
		return retContent, nil
	}

	// 5. Generate sub-queries
	missingStr := strings.Join(suff.MissingInformation, "\n - ")
	mg, err := dr.multiQueriesGen(ctx, question, query, missingStr, retContent)
	if err != nil {
		common.Warn("DeepResearcher: multi_queries_gen failed",
			zap.Error(err), zap.Int("depth", depth))
		return retContent, nil
	}

	if len(mg.Questions) == 0 {
		return retContent, nil
	}

	if callback != nil {
		var questionStrs []string
		for _, q := range mg.Questions {
			questionStrs = append(questionStrs, q.Question)
		}
		callback("Next step is to search for the following questions:</br> - " + strings.Join(questionStrs, "</br> - "))
	}

	// 6. Recurse in parallel
	var wg sync.WaitGroup
	results := make([]string, len(mg.Questions))
	mu := &sync.Mutex{}

	for i, qp := range mg.Questions {
		wg.Add(1)
		go func(idx int, q queryPair) {
			defer wg.Done()
			r, err := dr._research(ctx, chunkInfo, q.Question, q.Query, depth-1, callback)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				// Exceptions become string results (gather with return_exceptions)
				results[idx] = err.Error()
				common.Warn("DeepResearcher: sub-research failed",
					zap.Error(err), zap.String("question", q.Question))
				return
			}
			results[idx] = r
		}(i, qp)
	}

	// Wait with context cancellation support.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return retContent, ctx.Err()
	}

	// 7. Join results
	return strings.Join(results, "\n"), nil
}

// ──────────────────────────────────────────────────────────────────────
// Retrieval (KB + optional Web)
// ──────────────────────────────────────────────────────────────────────

// _retrieve_information does KB + optional web retrieval.
func (dr *DeepResearcher) _retrieve_information(ctx context.Context, query string) (map[string]interface{}, error) {
	kbinfos := map[string]interface{}{
		"total":    int64(0),
		"chunks":   []map[string]interface{}{},
		"doc_aggs": []interface{}{},
	}

	// 1. KB retrieval
	if dr.KBRetrieve != nil {
		resp, err := dr.KBRetrieve(ctx, query)
		if err != nil {
			common.Warn("DeepResearcher: KB retrieval error", zap.Error(err))
		}
		if resp != nil {
			chunks := make([]map[string]interface{}, len(resp.Chunks))
			copy(chunks, resp.Chunks)
			docAggs := make([]interface{}, 0, len(resp.DocAggs))
			for _, d := range resp.DocAggs {
				docAggs = append(docAggs, d)
			}
			kbinfos["chunks"] = chunks
			kbinfos["doc_aggs"] = docAggs
			kbinfos["total"] = resp.Total
		}
	}

	// 2. Web retrieval (Tavily)
	if dr.InternetEnabled && dr.TavilyAPIKey != "" {
		tavRes, err := dr.tavilyRetrieve(ctx, query)
		if err != nil {
			common.Warn("DeepResearcher: web retrieval error", zap.Error(err))
		} else if tavRes != nil {
			if chunks, ok := tavRes["chunks"].([]map[string]interface{}); ok {
				existing, _ := kbinfos["chunks"].([]map[string]interface{})
				kbinfos["chunks"] = append(existing, chunks...)
			}
			if aggs, ok := tavRes["doc_aggs"].([]interface{}); ok {
				existing, _ := kbinfos["doc_aggs"].([]interface{})
				kbinfos["doc_aggs"] = append(existing, aggs...)
			}
		}
	}

	// 3. Knowledge graph retrieval
	if useKG, _ := dr.PromptConfig["use_kg"].(bool); useKG && dr.ChatModel != nil && len(dr.KbIDs) > 0 {
		kgPipeline := kg.NewPipeline(dr.DocEngine, dr.KbIDs, dr.TenantIDs, query)
		kgPipeline.SetChatModel(dr.ChatModel)
		if dr.EmbModel != nil {
			kgPipeline.SetEmbModel(dr.EmbModel)
		}
		kgChunk, kgErr := kgPipeline.Retrieval(ctx)
		if kgErr != nil {
			common.Warn("DeepResearcher: KG retrieval failed", zap.Error(kgErr))
		} else if kgChunk != nil {
			if _, hasContent := kgChunk["content_with_weight"]; hasContent {
				if existingChunks, ok := kbinfos["chunks"].([]map[string]interface{}); ok {
					newChunks := make([]map[string]interface{}, 0, len(existingChunks)+1)
					newChunks = append(newChunks, kgChunk)
					newChunks = append(newChunks, existingChunks...)
					kbinfos["chunks"] = newChunks
				}
			}
		}
	}

	return kbinfos, nil
}

// tavilyRetrieve calls the Tavily Search API.
func (dr *DeepResearcher) tavilyRetrieve(ctx context.Context, query string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"query":        query,
		"api_key":      dr.TavilyAPIKey,
		"search_depth": "advanced",
		"max_results":  6,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tavily marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", dr.tavilyURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("tavily request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tavily read: %w", err)
	}

	var apiResp struct {
		Results []struct {
			URL     string  `json:"url"`
			Title   string  `json:"title"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("tavily parse: %w", err)
	}

	if len(apiResp.Results) == 0 {
		return nil, nil
	}

	chunks := make([]map[string]interface{}, 0, len(apiResp.Results))
	aggs := make([]interface{}, 0, len(apiResp.Results))
	for _, r := range apiResp.Results {
		id := strings.ReplaceAll(uuid.New().String(), "-", "")
		chunks = append(chunks, map[string]interface{}{
			"chunk_id":            id,
			"content_ltks":        tokenizeText(r.Content),
			"content_with_weight": r.Content,
			"doc_id":              id,
			"docnm_kwd":           r.Title,
			"kb_id":               []interface{}{},
			"important_kwd":       []interface{}{},
			"image_id":            "",
			"similarity":          r.Score,
			"vector_similarity":   1.0,
			"term_similarity":     0,
			"vector":              []interface{}{},
			"positions":           []interface{}{},
			"url":                 r.URL,
		})
		aggs = append(aggs, map[string]interface{}{
			"doc_name": r.Title,
			"doc_id":   id,
			"count":    1,
			"url":      r.URL,
		})
	}

	return map[string]interface{}{
		"chunks":   chunks,
		"doc_aggs": aggs,
	}, nil
}

// mergeChunkInfo merges kbinfos into chunkInfo, deduplicating by chunk_id / doc_id.
func (dr *DeepResearcher) mergeChunkInfo(
	chunkInfo map[string]interface{},
	kbinfos map[string]interface{},
) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	existingChunks, _ := chunkInfo["chunks"].([]map[string]interface{})

	// First retrieval — copy all keys from kbinfos
	if len(existingChunks) == 0 {
		for k := range chunkInfo {
			chunkInfo[k] = kbinfos[k]
		}
		return
	}

	newChunks, _ := kbinfos["chunks"].([]map[string]interface{})
	if len(newChunks) == 0 {
		return
	}

	// Build set of existing chunk IDs.
	seenChunkIDs := make(map[string]bool)
	for _, m := range existingChunks {
		if id, ok := m["chunk_id"].(string); ok {
			seenChunkIDs[id] = true
		}
	}

	// Append only new chunks.
	for _, m := range newChunks {
		id, _ := m["chunk_id"].(string)
		if id == "" {
			continue
		}
		if seenChunkIDs[id] {
			continue
		}
		seenChunkIDs[id] = true
		existingChunks = append(existingChunks, m)
	}

	chunkInfo["chunks"] = existingChunks

	// Merge doc_aggs (dedup by doc_id).
	newAggs, _ := kbinfos["doc_aggs"].([]interface{})
	if len(newAggs) == 0 {
		return
	}

	aggExisting, _ := chunkInfo["doc_aggs"].([]interface{})
	seenDocIDs := make(map[string]bool)
	for _, d := range aggExisting {
		if m, ok := d.(map[string]interface{}); ok {
			if id, ok := m["doc_id"].(string); ok {
				seenDocIDs[id] = true
			}
		}
	}
	for _, d := range newAggs {
		m, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := m["doc_id"].(string)
		if id == "" || seenDocIDs[id] {
			continue
		}
		seenDocIDs[id] = true
		aggExisting = append(aggExisting, d)
	}

	chunkInfo["doc_aggs"] = aggExisting

	// Accumulate total.
	existingTotal, _ := chunkInfo["total"].(int64)
	newTotal, _ := kbinfos["total"].(int64)
	chunkInfo["total"] = existingTotal + newTotal
}

// genJSON calls the LLM with a system prompt and retries on parse failure.
func (dr *DeepResearcher) genJSON(
	ctx context.Context,
	systemPrompt string,
	cfg *modelModule.ChatConfig,
	result interface{},
) error {
	maxRetry := 2
	var lastAns, lastErr string

	for attempt := 0; attempt < maxRetry; attempt++ {
		userPrompt := "Output:\n"
		if attempt > 0 && lastAns != "" && lastErr != "" {
			// Append correction to user message on retry
			userPrompt += fmt.Sprintf(
				"\nGenerated JSON is as following:\n%s\nBut exception while loading:\n%s\nPlease reconsider and correct it.",
				lastAns, lastErr,
			)
		}

		resp, err := dr.chatOnce(ctx, systemPrompt, userPrompt, cfg)
		if err != nil {
			return err
		}

		resp = cleanLLMResponse(resp)
		lastAns = resp

		repaired, rerr := jsonrepair.Repair(resp)
		if rerr != nil {
			repaired = resp
		}
		if err := json.Unmarshal([]byte(repaired), result); err != nil {
			lastErr = err.Error()
			common.Warn("genJSON: JSON parse failed, retrying",
				zap.Error(err), zap.Int("attempt", attempt))
			continue
		}
		return nil
	}
	return fmt.Errorf("genJSON: failed after %d attempts: %s", maxRetry, lastErr)
}

// sufficiencyCheck asks the LLM whether retrieved content is sufficient.
func (dr *DeepResearcher) sufficiencyCheck(
	ctx context.Context,
	question string,
	retContent string,
) (*sufficiencyResult, error) {
	systemPrompt := fmt.Sprintf(sufficiencyCheckTemplate, question, retContent)
	tempLow := 0.0
	cfg := &modelModule.ChatConfig{Temperature: &tempLow}

	var result sufficiencyResult
	if err := dr.genJSON(ctx, systemPrompt, cfg, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// multiQueriesGen asks the LLM to generate sub-queries from missing info.
func (dr *DeepResearcher) multiQueriesGen(
	ctx context.Context,
	originalQuestion string,
	originalQuery string,
	missingInfo string,
	retContent string,
) (*multiQueriesResult, error) {
	systemPrompt := fmt.Sprintf(multiQueriesGenTemplate,
		originalQuery, originalQuestion, retContent, missingInfo)
	tempLow := 0.4
	cfg := &modelModule.ChatConfig{Temperature: &tempLow}

	var result multiQueriesResult
	if err := dr.genJSON(ctx, systemPrompt, cfg, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// chatOnce is a single-turn LLM call. Returns the answer text.
func (dr *DeepResearcher) chatOnce(
	ctx context.Context,
	systemPrompt string,
	userPrompt string,
	cfg *modelModule.ChatConfig,
) (string, error) {
	if dr.ChatModel == nil || dr.ChatModel.ModelDriver == nil {
		return "", fmt.Errorf("DeepResearcher: no chat model configured")
	}
	modelName := ""
	if dr.ChatModel.ModelName != nil {
		modelName = *dr.ChatModel.ModelName
	}
	msgs := []modelModule.Message{
		modelModule.Message{Role: "system", Content: systemPrompt},
		modelModule.Message{Role: "user", Content: userPrompt},
	}
	resp, err := dr.ChatModel.ModelDriver.ChatWithMessages(
		modelName, msgs, dr.ChatModel.APIConfig, cfg,
	)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Answer == nil {
		return "", fmt.Errorf("empty response from chat model")
	}
	return *resp.Answer, nil
}

// cleanLLMResponse strips think tags, markdown fences, and trailing backticks.
var thinkTagRe = regexp.MustCompile(`(?s)^.*?</think>`)
var trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)
var cleanResponseRe = regexp.MustCompile(`(?s)(^.*?</think>|` + "```json\\n" + `|` + "```\\n*$" + `)`)
var trailingBacktickRe = regexp.MustCompile("```\\n*$")

func cleanLLMResponse(raw string) string {
	// Strip think tags, markdown fences
	raw = cleanResponseRe.ReplaceAllString(raw, "")

	// Also handle trailing ```` in case any remain after the regex pass
	raw = trailingBacktickRe.ReplaceAllString(raw, "")

	return strings.TrimSpace(raw)
}

// repairJSON: see metadata_filter.go:737 (canonical implementation).

// ChatModelMaxTokens returns the token budget for kb_prompt sizing (default 6000).
func (dr *DeepResearcher) ChatModelMaxTokens() int {
	return 6000
}

// kbPrompt formats retrieval results into knowledge blocks, truncating at 97% of maxTokens.
func kbPrompt(kbinfos map[string]interface{}, maxTokens int, hashID bool) []string {
	chunksRaw, _ := kbinfos["chunks"].([]map[string]interface{})
	if len(chunksRaw) == 0 {
		return nil
	}

	// Extract content strings.
	var knowledges []string
	for _, m := range chunksRaw {
		text := getMapString(m, "content")
		if text == "" {
			text = getMapString(m, "content_with_weight")
		}
		if text == "" {
			continue
		}
		knowledges = append(knowledges, text)
	}

	if len(knowledges) == 0 {
		return nil
	}

	// Truncate at 97% token budget.
	usedTokens := 0
	chunksNum := 0
	for i, c := range knowledges {
		usedTokens += tokenizer.NumTokensFromString(c)
		chunksNum++
		if usedTokens > int(float64(maxTokens)*0.97) {
			knowledges = knowledges[:i]
			common.Warn("kb_prompt: truncating chunks",
				zap.Int("kept", len(knowledges)),
				zap.Int("total", len(chunksRaw)))
			break
		}
	}

	// Format each chunk.
	knowledges = nil // reuse
	for i, m := range chunksRaw[:chunksNum] {

		id := i
		if hashID {
			// Hash chunk ID for stable int ID
			if rawID := getMapString(m, "id", "chunk_id"); rawID != "" {
				id = hashStrToInt(rawID, 500)
			}
		}

		cnt := fmt.Sprintf("\nID: %d", id)
		cnt += drawNode("Title", getMapString(m, "docnm_kwd", "document_name"))
		cnt += drawNode("URL", getMapString(m, "url"))

		if meta, ok := m["document_metadata"].(map[string]interface{}); ok {
			for k, v := range meta {
				cnt += drawNode(k, v)
			}
		}

		cnt += "\n└── Content:\n"
		text := getMapString(m, "content")
		if text == "" {
			text = getMapString(m, "content_with_weight")
		}
		cnt += text
		knowledges = append(knowledges, cnt)
	}

	return knowledges
}

// drawNode formats a key-value line with tree-drawing prefix.
func drawNode(k string, v interface{}) string {
	var line string
	switch val := v.(type) {
	case string:
		line = val
	case fmt.Stringer:
		line = val.String()
	default:
		line = fmt.Sprintf("%v", val)
	}
	if line == "" {
		return ""
	}
	// Collapse consecutive newlines into a single space
	var nb strings.Builder
	nb.Grow(len(line))
	inNewlines := false
	for _, r := range line {
		if r == '\n' {
			if !inNewlines {
				nb.WriteByte(' ')
				inNewlines = true
			}
		} else {
			nb.WriteRune(r)
			inNewlines = false
		}
	}
	line = nb.String()
	return fmt.Sprintf("\n├── %s: %s", k, line)
}

// hashStrToInt is an FNV-1a hash modulo mod.
func hashStrToInt(s string, mod int) int {
	if s == "" || mod <= 0 {
		return 0
	}
	var h uint64 = 14695981039346656037 // FNV offset basis
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211 // FNV prime
	}
	return int(h % uint64(mod))
}

// getMapString gets a string from a map, trying multiple keys.
func getMapString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// mapStringValue extracts a string value from a map by key.
func mapStringValue(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// chunksFromKBInfos extracts chunks list from kbinfos for counting.
func chunksFromKBInfos(kbinfos map[string]interface{}) []map[string]interface{} {
	if ch, ok := kbinfos["chunks"].([]map[string]interface{}); ok {
		return ch
	}
	return nil
}

// Ensure time is referenced (avoids unused import in some build configurations).
var _ = time.Now
