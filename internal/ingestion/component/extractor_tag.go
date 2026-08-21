package component

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	eschema "github.com/cloudwego/eino/schema"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const (
	// defaultMatchCoverageThreshold 阶段一非对称包含度阈值 (45%)
	defaultMatchCoverageThreshold = 0.45

	// defaultTopK 阶段一多样本聚合上限
	defaultTopK = 5

	// bgSmoothing 背景先验分布平滑常数
	bgSmoothing = 10.0
)

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

// MemoryTagIndex 预构建内存倒排索引 (构建完成后完全只读，天然并发安全)
type MemoryTagIndex struct {
	examples   []schema.TagLabel
	postings   map[string][]int   // word -> doc_ids (纯切片，无多余 map 堆开销)
	idfs       map[string]float64 // word -> idf
	exTotalIDF []float64          // doc_id -> sum(IDF(w))
	allTags    map[string]float64 // tag -> background prob (S=10)
}

func buildMemoryTagIndex(rawExamples []schema.TagLabel, tok tokenizer.Tokenizer) *MemoryTagIndex {
	if len(rawExamples) == 0 {
		return nil
	}

	cleanExamples := make([]schema.TagLabel, 0, len(rawExamples))
	exWordSets := make([]map[string]struct{}, 0, len(rawExamples))
	allTagCounts := make(map[string]int)
	totalTagCount := 0

	// 1. 过滤空内容样本、单样本标签去重与点号规范化 (深拷贝，严禁原地污染输入切片)
	for _, ex := range rawExamples {
		content := strings.TrimSpace(ex.Content)
		if content == "" {
			continue // 过滤空 Content 样本
		}
		tks, _ := tok.Tokenize(content)
		fields := strings.Fields(tks)
		if len(fields) == 0 {
			continue // 分词后为空也过滤
		}
		wordSet := make(map[string]struct{}, len(fields))
		for _, w := range fields {
			if w = strings.TrimSpace(w); w != "" {
				wordSet[w] = struct{}{}
			}
		}
		if len(wordSet) == 0 {
			continue
		}

		tagSet := make(map[string]struct{}, len(ex.Tags))
		cleanTags := make([]string, 0, len(ex.Tags))
		for _, t := range ex.Tags {
			t = strings.TrimSpace(strings.ReplaceAll(t, ".", "_"))
			if t != "" {
				if _, exists := tagSet[t]; !exists {
					tagSet[t] = struct{}{}
					cleanTags = append(cleanTags, t)
					allTagCounts[t]++
					totalTagCount++
				}
			}
		}

		cleanExamples = append(cleanExamples, schema.TagLabel{
			Content: content,
			Tags:    cleanTags,
		})
		exWordSets = append(exWordSets, wordSet)
	}

	N := float64(len(cleanExamples))
	if N == 0 || totalTagCount == 0 {
		return nil // 防御全部样本 Tags 为空的情况
	}

	docFreq := make(map[string]int)
	postings := make(map[string][]int)

	for i, wordSet := range exWordSets {
		// 不变量保证: 每个 docID 针对特定词只加入 postings[w] 一次
		for w := range wordSet {
			postings[w] = append(postings[w], i)
			docFreq[w]++ // 每篇样本只对 DF 贡献 1 次
		}
	}

	// 2. 标准平滑 IDF (全覆盖词自然接近 0，不加 +1.0)
	idfs := make(map[string]float64, len(docFreq))
	for w, df := range docFreq {
		idfs[w] = math.Log(1.0 + (N-float64(df)+0.5)/(float64(df)+0.5))
	}

	// 3. 预计算每个有效 Example 的总 IDF (短句 TF 恒 1，无需 exTF map，节省内存)
	exTotalIDF := make([]float64, len(cleanExamples))
	for i, wordSet := range exWordSets {
		var sum float64
		for w := range wordSet {
			sum += idfs[w]
		}
		exTotalIDF[i] = sum
	}

	// 4. 背景分布计算
	bgProportions := make(map[string]float64, len(allTagCounts))
	for t, count := range allTagCounts {
		bgProportions[t] = float64(count+1) / (float64(totalTagCount) + bgSmoothing)
	}

	return &MemoryTagIndex{
		examples:   cleanExamples,
		postings:   postings,
		idfs:       idfs,
		exTotalIDF: exTotalIDF,
		allTags:    bgProportions,
	}
}

// ----------------------------------------------------------------------
// 阶段一匹配主函数
// ----------------------------------------------------------------------

func matchAndTagChunk(
	chunk map[string]any,
	idx *MemoryTagIndex,
	tok tokenizer.Tokenizer,
	topN int,
) *schema.TaggedChunk {
	text := getChunkText(chunk)
	if text == "" || idx == nil || len(idx.allTags) == 0 {
		return nil
	}

	tokens, err := tok.Tokenize(text)
	if err != nil || tokens == "" {
		return nil
	}

	chunkWordSet := make(map[string]struct{})
	for _, w := range strings.Fields(tokens) {
		chunkWordSet[w] = struct{}{}
	}

	// 1. 倒排链表求交 (跳过 99% 无关样本)
	candidateInterIDF := make(map[int]float64)
	for w := range chunkWordSet {
		idf, exists := idx.idfs[w]
		if !exists || idf <= 0 {
			continue
		}
		for _, docID := range idx.postings[w] {
			candidateInterIDF[docID] += idf
		}
	}
	if len(candidateInterIDF) == 0 {
		return nil
	}

	// 2. 非对称覆盖率计算 (阈值 0.45)
	type candidateScore struct {
		docID    int
		coverage float64
	}
	var passed []candidateScore
	for docID, interIDF := range candidateInterIDF {
		exTotal := idx.exTotalIDF[docID]
		if exTotal <= 0 {
			continue
		}
		cov := interIDF / exTotal
		if cov >= defaultMatchCoverageThreshold {
			passed = append(passed, candidateScore{docID: docID, coverage: cov})
		}
	}
	if len(passed) == 0 {
		return nil
	}

	// 3. Top-K 样本按照 Coverage 加权聚合 (解决标签污染)
	sort.Slice(passed, func(i, j int) bool { return passed[i].coverage > passed[j].coverage })
	topK := min(defaultTopK, len(passed))

	tagWeightedCounts := make(map[string]float64)
	var totalWeightSum float64 // W = sum(Coverage_i)

	for i := 0; i < topK; i++ {
		cov := passed[i].coverage
		ex := idx.examples[passed[i].docID]
		if len(ex.Tags) == 0 {
			continue
		}
		totalWeightSum += cov // 外层累加，每篇命中文档贡献一次匹配权重
		for _, t := range ex.Tags {
			tagWeightedCounts[t] += cov // 加权累加
		}
	}

	if len(tagWeightedCounts) == 0 || totalWeightSum <= 0 {
		return nil
	}

	// 4. 打分: 纯 Lift 结合平均覆盖度保底 (无脆弱的乘法因子，高低频分布健康)
	avgCov := totalWeightSum / float64(topK)

	type tagScore struct {
		name  string
		score int
	}
	var scored []tagScore
	for t, weightedC := range tagWeightedCounts {
		bg := idx.allTags[t]
		if bg <= 0 {
			bg = 0.0001
		}
		pMatch := weightedC / totalWeightSum
		lift := pMatch / bg
		raw := max(lift, 5.0*avgCov)
		s := min(10, max(1, roundInt(raw)))
		scored = append(scored, tagScore{name: t, score: s})
	}

	if len(scored) == 0 {
		return nil
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > topN {
		scored = scored[:topN]
	}

	tagWeights := make(map[string]int, len(scored))
	matchedTags := make([]string, 0, len(scored))
	for _, ts := range scored {
		tagWeights[ts.name] = ts.score
		matchedTags = append(matchedTags, ts.name)
	}

	chunk[common.TAG_FLD] = tagWeights

	return &schema.TaggedChunk{
		Content:    text,
		Tags:       matchedTags,
		TagWeights: tagWeights,
	}
}

// ----------------------------------------------------------------------
// 缓存与调度加载链路 (全链路补齐 lang 透传与真实 Few-shot 兜底)
// ----------------------------------------------------------------------

const tagSourceCacheMax = 128

type boundedTagCache struct {
	mu     sync.Mutex
	cap    int
	items  map[string]*MemoryTagIndex
	recent []string
}

func newBoundedTagCache(cap int) *boundedTagCache {
	return &boundedTagCache{
		cap:   cap,
		items: make(map[string]*MemoryTagIndex, cap),
	}
}

func (c *boundedTagCache) load(key string) (*MemoryTagIndex, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.markRecentLocked(key)
	return v, true
}

func (c *boundedTagCache) store(key string, val *MemoryTagIndex) *MemoryTagIndex {
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

func (c *ExtractorComponent) runAutoTags(ctx context.Context, db *gorm.DB, in extractorInputs) ([]map[string]any, error) {
	indexed, ok := c.resolveTagSource(ctx, in.lang)
	if !ok || len(in.chunks) == 0 {
		common.Info("extractor tags: skipped",
			zap.Int("chunk_count", len(in.chunks)),
			zap.Bool("has_tag_source", ok),
			zap.String("llm_id", in.llmID),
		)
		return in.chunks, nil
	}

	topN := c.Param.Tags.TopN
	tok := tokenizer.New(in.lang)

	var examples []schema.TaggedChunk
	var docsToTag []map[string]any
	for _, d := range in.chunks {
		if ctx.Err() != nil {
			break
		}
		matched := matchAndTagChunk(d, indexed, tok, topN)
		if matched != nil {
			examples = append(examples, *matched)
		} else {
			docsToTag = append(docsToTag, d)
		}
	}

	if len(docsToTag) > 0 {
		driver, model, apiKey, baseURL, err := resolveExtractorChatTarget(ctx, db, in.llmID)
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
					llmTagChunk(ctx, db, inv, docsToTag[idx], indexed.allTags, examples, in.llmID, driver, model, apiKey, baseURL, topN, indexed)
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

func (c *ExtractorComponent) resolveTagSource(ctx context.Context, lang string) (*MemoryTagIndex, bool) {
	if c.Param.Tags.TagFileID == "" {
		return nil, false
	}
	return c.loadTagFileIndexed(ctx, lang)
}

func (c *ExtractorComponent) loadTagFileIndexed(ctx context.Context, lang string) (*MemoryTagIndex, bool) {
	f, err := dao.NewFileDAO().GetByID(ctx, dao.DB, c.Param.Tags.TagFileID)
	if err != nil || f == nil || f.Location == nil || *f.Location == "" {
		common.Warn(fmt.Sprintf("extractor tags: resolve tag_file_id %q: %v", c.Param.Tags.TagFileID, err))
		return nil, false
	}
	cacheKey := tagSourceFileCacheKey(f, lang)
	if cached, ok := tagSourceFileIndexCache.load(cacheKey); ok {
		common.Info("extractor tags: reused tag source file index",
			zap.String("file_id", c.Param.Tags.TagFileID),
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
	data, err := stg.Get(ctx, f.ParentID, *f.Location, tenantID)
	if err != nil {
		common.Warn(fmt.Sprintf("extractor tags: load tag source %q/%q: %v", f.ParentID, *f.Location, err))
		return nil, false
	}
	indexed, ok := buildIndexedTagSourceFromBytes(data, f.Name, lang)
	if !ok || indexed == nil {
		return nil, false // 防御 nil 存入 LRU 缓存
	}
	indexed = tagSourceFileIndexCache.store(cacheKey, indexed)
	common.Info("extractor tags: loaded tag source file",
		zap.String("file_id", c.Param.Tags.TagFileID),
		zap.String("bucket", f.ParentID),
		zap.String("key", *f.Location),
		zap.Int64("size", f.Size),
		zap.Int("bytes", len(data)),
	)
	return indexed, true
}

func buildIndexedTagSourceFromBytes(data []byte, filename, lang string) (*MemoryTagIndex, bool) {
	rawExamples, err := parseTagSourceByFilename(data, filename)
	if err != nil || len(rawExamples) == 0 {
		if err != nil {
			common.Warn(fmt.Sprintf("extractor tags: %v", err))
		}
		return nil, false
	}
	tok := tokenizer.New(lang)
	indexed := buildMemoryTagIndex(rawExamples, tok)
	if indexed == nil {
		return nil, false
	}
	return indexed, true
}

func tagSourceFileCacheKey(f *entity.File, lang string) string {
	location := ""
	if f.Location != nil {
		location = *f.Location
	}
	updateTime := int64(0)
	if f.UpdateTime != nil {
		updateTime = *f.UpdateTime
	}
	return fmt.Sprintf("tag-file:%s:%s:%s:%d:%d:%s", f.ID, f.ParentID, location, f.Size, updateTime, lang)
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
		if len(strings.Split(line, "\t")) == 2 {
			tab++
		}
	}
	if scanner.Err() != nil {
		common.Warn(fmt.Sprintf("extractor tags: delimiter scan: %v", scanner.Err()))
	}
	if tab >= comma {
		return "\t"
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

// ----------------------------------------------------------------------
// 阶段二 LLM 真实样本兜底与响应点号清洗
// ----------------------------------------------------------------------

func llmTagChunk(
	ctx context.Context,
	db *gorm.DB,
	inv extractorChatInvoker,
	chunk map[string]any,
	allTags map[string]float64,
	examples []schema.TaggedChunk,
	llmID, driver, model, apiKey, baseURL string,
	topN int,
	idx *MemoryTagIndex,
) {
	text := getChunkText(chunk)
	if text == "" {
		return
	}

	if cached := getTaggerLLMCache(ctx, llmID, text, allTags, topN); cached != nil {
		chunk[common.TAG_FLD] = cached
		return
	}

	var picked []schema.TaggedChunk
	if len(examples) > 2 {
		picked = randomChoices(examples, 2)
	} else if len(examples) > 0 {
		picked = examples
	} else if idx != nil && len(idx.examples) > 0 {
		// 冷启动兜底: 从真实参考样本中采样，彻底废弃不在 TAG SET 中的假样本
		sampleCount := min(2, len(idx.examples))
		for i := 0; i < sampleCount; i++ {
			ex := idx.examples[i]
			weights := make(map[string]int, len(ex.Tags))
			p := 1.0 / float64(max(1, len(ex.Tags)))
			for _, t := range ex.Tags {
				lift := p / max(1e-6, idx.allTags[t])
				weights[t] = min(10, max(1, roundInt(lift)))
			}
			picked = append(picked, schema.TaggedChunk{
				Content:    ex.Content,
				Tags:       ex.Tags,
				TagWeights: weights,
			})
		}
	}

	tagNames := sortedTagNames(allTags)
	tagSetStr := strings.Join(tagNames, ", ")
	prompt := buildTaggerPrompt(topN, tagSetStr, picked, text)

	msgs := []eschema.Message{
		{Role: eschema.System, Content: prompt},
		{Role: eschema.User, Content: "Output:"},
	}
	// Trim the prompt to the model's context window before sending. The
	// system prompt embeds the full chunk text, the entire tag set and up
	// to two full examples, so oversized chunks or tag files would
	// otherwise be rejected by the provider (context length exceeded).
	// Mirrors Python's message_fit_in in content_tagging (generator.py:331).
	fitted, fitErr := fitExtractorMessages(ctx, db, llmID, msgs)
	if fitErr != nil {
		common.Warn("extractor tags: skipping LLM tagging, message fitting failed", zap.Error(fitErr))
		return
	}
	msgs = fitted

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
		setTaggerLLMCache(ctx, llmID, text, allTags, topN, result)
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
	raw = strings.TrimSpace(common.StripThinkTrailing(raw))
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
		cleanKey := strings.ReplaceAll(strings.TrimSpace(k), ".", "_")
		if score > 0 && cleanKey != "" {
			result[cleanKey] = score
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

func getTaggerLLMCache(ctx context.Context, llmID, text string, allTags map[string]float64, topN int) map[string]int {
	client := redis.Get()
	if client == nil {
		return nil
	}
	key := taggerCacheKey(llmID, text, allTags, topN)
	data, err := client.Get(ctx, key)
	if err != nil || data == "" {
		return nil
	}
	var result map[string]int
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil
	}
	return result
}

func setTaggerLLMCache(ctx context.Context, llmID, text string, allTags map[string]float64, topN int, result map[string]int) {
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
	client.Set(ctx, key, string(data), 24*time.Hour)
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

