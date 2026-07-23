package component

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/cespare/xxhash/v2"
	eschema "github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const matchOverlapThreshold = 0.5

const taggerLLMConcurrency = 8

var taggerTimeout = 60 * time.Second

const taggerPromptTmpl = `## Role
You are a text analyzer.

## Task
Add tags (labels) to a given piece of text content based on the examples and the entire tag set.

## Steps
- Review the tag/label set.
- Review examples which all consist of both text content and assigned tags with relevance score in JSON format.
- Summarize the text content, and tag it with the top %d most relevant tags from the set of tags/labels and the corresponding relevance score.

## Requirements
- The tags MUST be from the tag set.
- The output MUST be in JSON format only, the key is tag and the value is its relevance score.
- The relevance score must range from 1 to 10.
- Output keywords ONLY.

# TAG SET
%s

%s
# Real Data
### Text Content
%s

Output:`

const taggerExampleBlock = `# Examples %d
### Text Content
%s

Output:
%s

`

type indexedTagSource struct {
	examples  []schema.TagLabel
	allTags   map[string]float64
	tagTokens [][]string
}

const tagSourceCacheMax = 128

type boundedTagCache struct {
	mu     sync.Mutex
	cap    int
	items  map[string]*indexedTagSource
	recent []string
}

func newBoundedTagCache(cap int) *boundedTagCache {
	return &boundedTagCache{
		cap:   cap,
		items: make(map[string]*indexedTagSource, cap),
	}
}

func (c *boundedTagCache) load(key string) (*indexedTagSource, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.markRecentLocked(key)
	return v, true
}

func (c *boundedTagCache) store(key string, val *indexedTagSource) *indexedTagSource {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.items[key]; ok {
		return existing
	}
	c.items[key] = val
	c.recent = append(c.recent, key)
	for len(c.items) > c.cap {
		oldest := c.recent[0]
		c.recent = c.recent[1:]
		delete(c.items, oldest)
	}
	return val
}

func (c *boundedTagCache) markRecentLocked(key string) {
	for i, k := range c.recent {
		if k == key {
			c.recent = append(c.recent[:i], c.recent[i+1:]...)
			c.recent = append(c.recent, k)
			break
		}
	}
}

var tagSourceFileIndexCache = newBoundedTagCache(tagSourceCacheMax)

func (c *ExtractorComponent) runAutoTags(ctx context.Context, in extractorInputs) ([]map[string]any, error) {
	indexed, ok := c.resolveTagSource(ctx)
	if !ok || len(in.chunks) == 0 {
		common.Info("extractor tags: skipped",
			zap.Int("chunk_count", len(in.chunks)),
			zap.Bool("has_tag_source", ok),
			zap.String("llm_id", in.llmID),
		)
		return in.chunks, nil
	}
	if len(indexed.examples) == 0 || len(indexed.allTags) == 0 {
		common.Info("extractor tags: empty indexed source",
			zap.Int("chunk_count", len(in.chunks)),
			zap.Int("example_count", len(indexed.examples)),
			zap.Int("all_tag_count", len(indexed.allTags)),
			zap.String("llm_id", in.llmID),
		)
		return in.chunks, nil
	}

	topN := c.Param.AutoTags

	var examples []schema.TaggedChunk
	var docsToTag []map[string]any
	for _, d := range in.chunks {
		if ctx.Err() != nil {
			break
		}
		matched := matchAndTagChunk(d, indexed.examples, indexed.tagTokens, indexed.allTags, topN)
		if matched != nil {
			examples = append(examples, *matched)
		} else if in.llmID != "" {
			docsToTag = append(docsToTag, d)
		}
	}

	if len(docsToTag) > 0 && in.llmID != "" {
		driver, model, apiKey, baseURL, err := resolveExtractorChatTarget(ctx, in.llmID)
		if err != nil {
			common.Warn("extractor tag: resolve model failed, skipping LLM tagging", zap.Error(err))
		}
		if driver != "" && model != "" {
			inv := getExtractorChatInvoker()
			sem := make(chan struct{}, taggerLLMConcurrency)
			var wg sync.WaitGroup

			for i := range docsToTag {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-ctx.Done():
						return
					}
					llmTagChunk(ctx, inv, docsToTag[idx], indexed.allTags, examples, in.llmID, driver, model, apiKey, baseURL, topN)
				}(i)
			}
			wg.Wait()
		}
	}

	taggedCount := 0
	for _, chunk := range in.chunks {
		if chunk[common.TAG_FLD] != nil {
			taggedCount++
		}
	}
	common.Info("extractor tags: completed",
		zap.Int("chunk_count", len(in.chunks)),
		zap.Int("example_count", len(indexed.examples)),
		zap.Int("all_tag_count", len(indexed.allTags)),
		zap.Int("phase1_match_count", len(examples)),
		zap.Int("phase2_candidate_count", len(docsToTag)),
		zap.Int("tagged_chunk_count", taggedCount),
		zap.Bool("has_llm", in.llmID != ""),
	)

	return in.chunks, nil
}

func (c *ExtractorComponent) resolveTagSource(ctx context.Context) (*indexedTagSource, bool) {
	if c.Param.TagFileID == "" {
		return nil, false
	}
	return c.loadTagFileIndexed(ctx)
}

func (c *ExtractorComponent) loadTagFileIndexed(ctx context.Context) (*indexedTagSource, bool) {
	f, err := dao.NewFileDAO().GetByID(c.Param.TagFileID)
	if err != nil || f == nil || f.Location == nil || *f.Location == "" {
		common.Warn(fmt.Sprintf("extractor tags: resolve tag_file_id %q: %v", c.Param.TagFileID, err))
		return nil, false
	}
	cacheKey := tagSourceFileCacheKey(f)
	if cached, ok := tagSourceFileIndexCache.load(cacheKey); ok {
		common.Info("extractor tags: reused tag source file index",
			zap.String("file_id", c.Param.TagFileID),
			zap.String("bucket", f.ParentID),
			zap.String("key", *f.Location),
		)
		return cached, true
	}
	stg := resolveStorage()
	if stg == nil {
		common.Warn("extractor tags: no storage backend registered")
		return nil, false
	}
	tenantID := globals.GlobalOrInput(ctx, nil, "tenant_id", "")
	data, err := stg.Get(f.ParentID, *f.Location, tenantID)
	if err != nil {
		common.Warn(fmt.Sprintf("extractor tags: load tag source %q/%q: %v", f.ParentID, *f.Location, err))
		return nil, false
	}
	indexed, ok := buildIndexedTagSourceFromBytes(data, f.Name)
	if !ok {
		return nil, false
	}
	indexed = tagSourceFileIndexCache.store(cacheKey, indexed)
	common.Info("extractor tags: loaded tag source file",
		zap.String("file_id", c.Param.TagFileID),
		zap.String("bucket", f.ParentID),
		zap.String("key", *f.Location),
		zap.Int64("size", f.Size),
		zap.Int("bytes", len(data)),
	)
	return indexed, true
}

func buildIndexedTagSourceFromBytes(data []byte, filename string) (*indexedTagSource, bool) {
	examples, err := parseTagSourceByFilename(data, filename)
	if err != nil {
		common.Warn(fmt.Sprintf("extractor tags: %v", err))
		return nil, false
	}
	return buildIndexedTagSourceFromExamples(examples), true
}

// parseTagSourceByFilename mirrors rag/app/tag.py chunk(): the format is chosen
// by the file extension, and only .xlsx/.xls, .txt and .csv are supported. Any
// other extension (including no extension) is rejected, matching Python's
// NotImplementedError for unsupported formats. xlsx is parsed per-sheet (2
// columns, no header, multiple sheets); .csv uses a quote-aware reader; .txt
// uses the delimiter-detecting reader.
func parseTagSourceByFilename(data []byte, filename string) ([]schema.TagLabel, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".xlsx", ".xls":
		return parseXLSXTagSource(data), nil
	case ".csv":
		return parseCSVQuoteAwareReader(bytes.NewReader(data)), nil
	case ".txt":
		delimiter := detectCSVDelimiterBytes(data)
		return parseCSVTagSourceReader(bytes.NewReader(data), delimiter), nil
	default:
		return nil, fmt.Errorf("unsupported tag source extension %q: only .xlsx, .txt and .csv are supported", filepath.Ext(filename))
	}
}

func buildIndexedTagSourceFromExamples(examples []schema.TagLabel) *indexedTagSource {
	return &indexedTagSource{
		examples:  examples,
		allTags:   buildAllTagsProportions(examples),
		tagTokens: preTokenizeExamples(examples),
	}
}

func tagSourceFileCacheKey(f *entity.File) string {
	location := ""
	if f.Location != nil {
		location = *f.Location
	}
	updateTime := int64(0)
	if f.UpdateTime != nil {
		updateTime = *f.UpdateTime
	}
	return fmt.Sprintf("tag-file:%s:%s:%s:%d:%d", f.ID, f.ParentID, location, f.Size, updateTime)
}

func parseCSVTagSource(text string) []schema.TagLabel {
	return parseCSVTagSourceBytes([]byte(text))
}

func parseCSVTagSourceBytes(data []byte) []schema.TagLabel {
	return parseCSVTagSourceReader(bytes.NewReader(data), detectCSVDelimiterBytes(data))
}

// parseCSVTagSourceReader mirrors rag/app/tag.py's txt parsing: lines that do
// not split into exactly two columns are accumulated as body text and prepended
// to the next tagged line. The second column holds comma-separated tags.
func parseCSVTagSourceReader(r io.Reader, delimiter string) []schema.TagLabel {
	scanner := newTagSourceScannerFromReader(r, scanBufferMax)
	result := make([]schema.TagLabel, 0)
	content := ""
	appendLine := func(s string) {
		if content == "" {
			content = s
		} else {
			content += "\n" + s
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		arr := strings.Split(line, delimiter)
		if len(arr) != 2 {
			appendLine(line)
			continue
		}
		appendLine(arr[0])
		tags := splitAndTrim(arr[1], ",")
		result = append(result, schema.TagLabel{Content: content, Tags: tags})
		content = ""
	}
	if scanner.Err() != nil {
		common.Warn(fmt.Sprintf("extractor tags: parse tag source: %v", scanner.Err()))
	}
	return result
}

// parseCSVQuoteAwareReader mirrors rag/app/tag.py's .csv path: each line is
// parsed with encoding/csv (so quoted fields containing the delimiter are
// handled), lines that do not yield exactly two non-empty columns are
// accumulated as body text and prepended to the next tagged line, and the
// second column holds comma-separated tags.
func parseCSVQuoteAwareReader(r io.Reader) []schema.TagLabel {
	scanner := newTagSourceScannerFromReader(r, scanBufferMax)
	result := make([]schema.TagLabel, 0)
	content := ""
	appendLine := func(s string) {
		if content == "" {
			content = s
		} else {
			content += "\n" + s
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		rec, err := csv.NewReader(strings.NewReader(line)).Read()
		if err != nil {
			appendLine(line)
			continue
		}
		row := stripEmptyFields(rec)
		if len(row) != 2 {
			appendLine(line)
			continue
		}
		appendLine(row[0])
		tags := splitAndTrim(row[1], ",")
		result = append(result, schema.TagLabel{Content: content, Tags: tags})
		content = ""
	}
	if scanner.Err() != nil {
		common.Warn(fmt.Sprintf("extractor tags: parse csv tag source: %v", scanner.Err()))
	}
	return result
}

// parseXLSXTagSource mirrors rag/app/tag.py's .xlsx path: every sheet is read
// with no header, and each row contributes a (content, tags) pair from its
// first and second non-empty cells. The second cell holds comma-separated tags.
func parseXLSXTagSource(data []byte) []schema.TagLabel {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		common.Warn(fmt.Sprintf("extractor tags: open xlsx tag source: %v", err))
		return nil
	}
	defer f.Close()

	result := make([]schema.TagLabel, 0)
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			common.Warn(fmt.Sprintf("extractor tags: read xlsx sheet %q: %v", sheet, err))
			continue
		}
		for _, row := range rows {
			var cells []string
			for _, c := range row {
				if c = strings.TrimSpace(c); c != "" {
					cells = append(cells, c)
				}
			}
			if len(cells) < 2 {
				continue
			}
			tags := splitAndTrim(cells[1], ",")
			result = append(result, schema.TagLabel{Content: cells[0], Tags: tags})
		}
	}
	return result
}

func stripEmptyFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

const scanBufferMax = 1 << 20

func detectCSVDelimiterBytes(data []byte) string {
	comma, tab := 0, 0
	scanner := newTagSourceScanner(bytes.NewReader(data), len(data))
	for scanner.Scan() {
		line := scanner.Text()
		if len(strings.Split(line, ",")) == 2 {
			comma++
		}
		if len(strings.Split(line, "	")) == 2 {
			tab++
		}
	}
	if scanner.Err() != nil {
		common.Warn(fmt.Sprintf("extractor tags: delimiter scan: %v", scanner.Err()))
	}
	if tab >= comma {
		return "	"
	}
	return ","
}

func newTagSourceScanner(r io.Reader, dataLen int) *bufio.Scanner {
	maxToken := dataLen + 1
	if maxToken < 64*1024 {
		maxToken = 64 * 1024
	}
	if maxToken > scanBufferMax {
		maxToken = scanBufferMax
	}
	return newTagSourceScannerFromReader(r, maxToken)
}

func newTagSourceScannerFromReader(r io.Reader, maxTokens ...int) *bufio.Scanner {
	maxToken := 64 * 1024
	if len(maxTokens) > 0 && maxTokens[0] > maxToken {
		maxToken = maxTokens[0]
	}
	initBuf := min(64*1024, maxToken)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, initBuf), maxToken)
	return scanner
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildAllTagsProportions(tagSource []schema.TagLabel) map[string]float64 {
	tagCount := make(map[string]int)
	total := 0
	for _, ex := range tagSource {
		for _, t := range ex.Tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			tagCount[t]++
			total++
		}
	}
	S := 1000.0
	proportions := make(map[string]float64, len(tagCount))
	for tag, c := range tagCount {
		proportions[tag] = float64(c+1) / (float64(total) + S)
	}
	return proportions
}

func preTokenizeExamples(tagSource []schema.TagLabel) [][]string {
	out := make([][]string, len(tagSource))
	for i := range tagSource {
		tokens, err := tokenizer.Tokenize(tagSource[i].Content)
		if err == nil && tokens != "" {
			out[i] = strings.Fields(tokens)
		}
	}
	return out
}

func matchAndTagChunk(
	chunk map[string]any,
	tagSource []schema.TagLabel,
	tagTokens [][]string,
	allTags map[string]float64,
	topN int,
) *schema.TaggedChunk {
	text := getChunkText(chunk)
	if text == "" {
		return nil
	}
	tokens, err := tokenizer.Tokenize(text)
	if err != nil || tokens == "" {
		return nil
	}
	chunkWords := strings.Fields(tokens)

	var best *schema.TagLabel
	var bestScore float64
	for i := range tagSource {
		exWords := tagTokens[i]
		if exWords == nil {
			continue
		}
		score := jaccardOverlap(chunkWords, exWords)
		if score > bestScore && score >= matchOverlapThreshold {
			bestScore = score
			ex := tagSource[i]
			best = &ex
		}
	}
	if best == nil {
		return nil
	}

	S := 1000.0
	cnt := float64(len(best.Tags))
	type tagScore struct {
		name  string
		score int
	}
	var scored []tagScore
	for _, t := range best.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		bg := allTags[t]
		if bg <= 0 {
			bg = 0.0001
		}
		s := roundInt(0.1 * 2.0 / (cnt + S) / max(1e-6, bg))
		if s > 0 {
			scored = append(scored, tagScore{name: t, score: s})
		}
	}

	if len(scored) == 0 {
		return nil
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > topN {
		scored = scored[:topN]
	}

	tagWeights := make(map[string]int, len(scored))
	for _, ts := range scored {
		tagWeights[strings.ReplaceAll(ts.name, ".", "_")] = ts.score
	}
	// Store as an object, not a JSON string: the ES index maps tag_feas as
	// type "rank_features", which requires an object of numeric values. A
	// string would make every bulk insert fail and drop the whole chunk.
	chunk[common.TAG_FLD] = tagWeights

	return &schema.TaggedChunk{
		Content:    text,
		Tags:       best.Tags,
		TagWeights: tagWeights,
	}
}

func jaccardOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, w := range a {
		setA[w] = struct{}{}
	}
	setB := make(map[string]struct{}, len(b))
	for _, w := range b {
		setB[w] = struct{}{}
	}
	intersection := 0
	for w := range setA {
		if _, ok := setB[w]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func getChunkText(chunk map[string]any) string {
	if v, ok := chunk["content_with_weight"].(string); ok && v != "" {
		return v
	}
	if v, ok := chunk["text"].(string); ok && v != "" {
		return v
	}
	return ""
}

func roundInt(f float64) int {
	if f < 0 {
		return int(f - 0.5)
	}
	return int(f + 0.5)
}

func llmTagChunk(
	ctx context.Context,
	inv extractorChatInvoker,
	chunk map[string]any,
	allTags map[string]float64,
	examples []schema.TaggedChunk,
	llmID, driver, model, apiKey, baseURL string,
	topN int,
) {
	text := getChunkText(chunk)
	if text == "" {
		return
	}

	if cached := getTaggerLLMCache(llmID, text, allTags, topN); cached != nil {
		chunk[common.TAG_FLD] = cached
		return
	}

	var picked []schema.TaggedChunk
	if len(examples) > 2 {
		picked = randomChoices(examples, 2)
	} else if len(examples) > 0 {
		picked = examples
	}
	if len(picked) == 0 {
		picked = []schema.TaggedChunk{{Content: "This is an example", TagWeights: map[string]int{"example": 1}}}
	}

	tagNames := sortedTagNames(allTags)
	tagSetStr := strings.Join(tagNames, ", ")
	prompt := buildTaggerPrompt(topN, tagSetStr, picked, text)

	msgs := []eschema.Message{
		{Role: eschema.System, Content: prompt},
		{Role: eschema.User, Content: "Output:"},
	}

	temperature := 0.5
	var result map[string]int
	timeoutErr := runtime.WithTimeout(ctx, taggerTimeout, func(timeoutCtx context.Context) error {
		resp, err := inv.Chat(timeoutCtx, extractorChatRequest{
			Driver:      driver,
			ModelName:   model,
			APIKey:      apiKey,
			BaseURL:     baseURL,
			Messages:    msgs,
			Temperature: &temperature,
		})
		if err != nil {
			common.Error("extractor tags: LLM call failed", err)
			return nil
		}
		result = parseTaggerResponse(resp.Content, topN)
		return nil
	})
	if timeoutErr != nil {
		common.Error("extractor tags: LLM timeout", timeoutErr)

	}

	if len(result) > 0 {
		chunk[common.TAG_FLD] = result
		setTaggerLLMCache(llmID, text, allTags, topN, result)
	}
}

func buildTaggerPrompt(topN int, tagSetStr string, examples []schema.TaggedChunk, text string) string {
	var examplesBlock strings.Builder
	for i, ex := range examples {
		tagsJSON, _ := json.Marshal(ex.TagWeights)
		examplesBlock.WriteString(fmt.Sprintf(taggerExampleBlock, i, ex.Content, string(tagsJSON)))
	}
	return fmt.Sprintf(taggerPromptTmpl, topN, tagSetStr, examplesBlock.String(), text)
}

func parseTaggerResponse(raw string, topN int) map[string]int {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "</think>"); idx >= 0 {
		raw = strings.TrimSpace(raw[idx+len("</think>"):])
	}
	if strings.Contains(raw, "**ERROR**") {
		common.Warn("extractor tags: LLM returned **ERROR**")
		return nil
	}

	obj, ok := tryParseJSONObject(raw)
	if !ok {
		obj = jsonRepairExtract(raw)
		if obj == nil {
			return nil
		}
	}

	result := make(map[string]int, len(obj))
	for k, v := range obj {
		score := 0
		switch n := v.(type) {
		case float64:
			score = int(n)
		case int:
			score = n
		}
		if score > 0 {
			result[k] = score
		}
	}

	if len(result) > topN {
		type kv struct {
			k string
			v int
		}
		sorted := make([]kv, 0, len(result))
		for k, v := range result {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
		result = make(map[string]int, topN)
		for i := 0; i < topN && i < len(sorted); i++ {
			result[sorted[i].k] = sorted[i].v
		}
	}
	return result
}

func jsonRepairExtract(raw string) map[string]any {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil
	}
	candidate := raw[start : end+1]
	obj, ok := tryParseJSONObject(candidate)
	if !ok {
		return nil
	}
	return obj
}

func taggerCacheKey(llmID, text string, allTags map[string]float64, topN int) string {
	hasher := xxhash.New()
	hasher.Write([]byte(llmID))
	hasher.Write([]byte("\x00"))
	hasher.Write([]byte(text))
	hasher.Write([]byte("\x00"))
	tagNames := sortedTagNames(allTags)
	hasher.Write([]byte(strings.Join(tagNames, ",")))
	hasher.Write([]byte("\x00"))
	hasher.Write([]byte(fmt.Sprintf("%d", topN)))
	return fmt.Sprintf("tagger:%x", hasher.Sum64())
}

func getTaggerLLMCache(llmID, text string, allTags map[string]float64, topN int) map[string]int {
	client := redis.Get()
	if client == nil {
		return nil
	}
	key := taggerCacheKey(llmID, text, allTags, topN)
	data, err := client.Get(key)
	if err != nil || data == "" {
		return nil
	}
	var result map[string]int
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil
	}
	return result
}

func setTaggerLLMCache(llmID, text string, allTags map[string]float64, topN int, result map[string]int) {
	if result == nil {
		return
	}
	client := redis.Get()
	if client == nil {
		return
	}
	key := taggerCacheKey(llmID, text, allTags, topN)
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	client.Set(key, string(data), 24*time.Hour)
}

func sortedTagNames(allTags map[string]float64) []string {
	out := make([]string, 0, len(allTags))
	for t := range allTags {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func randomChoices(slice []schema.TaggedChunk, k int) []schema.TaggedChunk {
	if len(slice) == 0 {
		return nil
	}
	out := make([]schema.TaggedChunk, k)
	for i := range k {
		out[i] = slice[rand.IntN(len(slice))]
	}
	return out
}
