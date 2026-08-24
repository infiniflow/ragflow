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
	"strconv"
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
	// defaultMatchCoverageThreshold is the asymmetric coverage threshold for Phase 1 (55%).
	defaultMatchCoverageThreshold = 0.55

	// singleTokenCoverageThreshold is the full-coverage threshold required for 1-token rules (99.9%).
	singleTokenCoverageThreshold = 0.999

	// singleTokenMinIDF is the minimum total IDF required for 1-token rules to match.
	singleTokenMinIDF = 1.5

	// defaultTopK is the maximum number of matched example candidates to aggregate in Phase 1.
	defaultTopK = 5

	// bgSmoothing is the Dirichlet smoothing constant for background prior tag probabilities.
	bgSmoothing = 10.0

	// rankDecayTierDropRelativeThreshold is the relative drop threshold to descend an effective rank tier.
	rankDecayTierDropRelativeThreshold = 0.05

	// rankDecayBaseMax is the initial rank base score for the top effective rank.
	rankDecayBaseMax = 8.5

	// rankDecayBaseMin is the floor for rank base score.
	rankDecayBaseMin = 3.0

	// rankDecayStep is the score decay per effective rank tier.
	rankDecayStep = 2.0

	// rankDecayWeightPower is the power exponent applied to relative tag weight.
	rankDecayWeightPower = 0.4
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

// MemoryTagIndex is an in-memory inverted index of tag examples (immutable after construction, safe for concurrent use).
type MemoryTagIndex struct {
	examples     []schema.TagLabel
	postings     map[string][]int   // word -> doc_ids (compact slice without per-doc map overhead)
	idfs         map[string]float64 // word -> idf
	exTotalIDF   []float64          // doc_id -> sum(IDF(w))
	exTokenCount []int              // doc_id -> number of unique tokens
	allTags      map[string]float64 // tag -> background prob (S=10)
}

func buildMemoryTagIndex(rawExamples []schema.TagLabel, tok tokenizer.Tokenizer) *MemoryTagIndex {
	if len(rawExamples) == 0 {
		return nil
	}

	cleanExamples := make([]schema.TagLabel, 0, len(rawExamples))
	exWordSets := make([]map[string]struct{}, 0, len(rawExamples))
	allTagCounts := make(map[string]int)
	totalTagCount := 0

	// 1. Filter empty contents, deduplicate tags per sample, and normalize dots with deep copies.
	for _, ex := range rawExamples {
		content := strings.TrimSpace(ex.Content)
		if content == "" {
			continue // Skip empty content.
		}
		tks, err := tok.Tokenize(content)
		if err != nil {
			common.Warn(fmt.Sprintf("extractor tags: tokenize example failed: %v", err))
			continue
		}
		fields := strings.Fields(tks)
		if len(fields) == 0 {
			continue // Skip samples with no tokens.
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
				}
			}
		}
		if len(cleanTags) == 0 {
			continue // Skip examples with no usable tags.
		}
		for _, t := range cleanTags {
			allTagCounts[t]++
			totalTagCount++
		}

		cleanExamples = append(cleanExamples, schema.TagLabel{
			Content: content,
			Tags:    cleanTags,
		})
		exWordSets = append(exWordSets, wordSet)
	}

	N := float64(len(cleanExamples))
	if N == 0 || totalTagCount == 0 {
		return nil // Guard against all samples having empty tags.
	}

	docFreq := make(map[string]int)
	postings := make(map[string][]int)

	for i, wordSet := range exWordSets {
		// Invariant: each docID is added to postings[w] at most once.
		for w := range wordSet {
			postings[w] = append(postings[w], i)
			docFreq[w]++ // Each document contributes at most 1 to document frequency.
		}
	}

	// 2. Standard smoothed IDF (terms covering all documents smoothly approach 0).
	idfs := make(map[string]float64, len(docFreq))
	for w, df := range docFreq {
		idfs[w] = math.Log(1.0 + (N-float64(df)+0.5)/(float64(df)+0.5))
	}

	// 3. Precompute total IDF and token count per valid example (short-text TF=1, avoiding per-example TF maps).
	exTotalIDF := make([]float64, len(cleanExamples))
	exTokenCount := make([]int, len(cleanExamples))
	for i, wordSet := range exWordSets {
		var sum float64
		for w := range wordSet {
			sum += idfs[w]
		}
		exTotalIDF[i] = sum
		exTokenCount[i] = len(wordSet)
	}

	// 4. Background prior probability distribution (smoothed Dirichlet prior).
	alpha := bgSmoothing / float64(len(allTagCounts))
	bgProportions := make(map[string]float64, len(allTagCounts))
	for t, count := range allTagCounts {
		bgProportions[t] = (float64(count) + alpha) / (float64(totalTagCount) + bgSmoothing)
	}

	return &MemoryTagIndex{
		examples:     cleanExamples,
		postings:     postings,
		idfs:         idfs,
		exTotalIDF:   exTotalIDF,
		exTokenCount: exTokenCount,
		allTags:      bgProportions,
	}
}

// OrderedTagWeights represents a tag-to-weight mapping that preserves
// score-descending order when marshaled to JSON for Elasticsearch storage.
type OrderedTagWeights map[string]int

func (m OrderedTagWeights) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	type kv struct {
		k string
		v int
	}
	kvs := make([]kv, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].v != kvs[j].v {
			return kvs[i].v > kvs[j].v
		}
		return kvs[i].k < kvs[j].k
	})

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, item := range kvs {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, _ := json.Marshal(item.k)
		buf.Write(keyBytes)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(item.v))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ----------------------------------------------------------------------
// Phase 1 local matching
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

	chunkWordFreq := make(map[string]int)
	for _, w := range strings.Fields(tokens) {
		if w = strings.TrimSpace(w); w != "" {
			chunkWordFreq[w]++
		}
	}

	// 1. Inverted index lookup (skips irrelevant samples).
	candidateInterIDF := make(map[int]float64)
	candidateTFScore := make(map[int]float64)
	candidateMatchedTokens := make(map[int]int)
	for w, freq := range chunkWordFreq {
		idf, exists := idx.idfs[w]
		if !exists || idf <= 1e-6 {
			continue
		}
		tfBoost := 1.0 + 0.3*math.Log(1.0+float64(freq))
		for _, docID := range idx.postings[w] {
			candidateInterIDF[docID] += idf          // Pure IDF sum, ensures coverage <= 1.0
			candidateTFScore[docID] += idf * tfBoost // Saliency weighted by TF
			candidateMatchedTokens[docID]++
		}
	}
	if len(candidateInterIDF) == 0 {
		return nil
	}

	// 2. Compute asymmetric coverage (Coverage(E) = InterIDF / ExTotalIDF in [0.0, 1.0]).
	type candidateScore struct {
		docID    int
		coverage float64
		tfWeight float64
	}
	passed := make([]candidateScore, 0, len(candidateInterIDF))
	for docID, interIDF := range candidateInterIDF {
		exTotal := idx.exTotalIDF[docID]
		if exTotal <= 0 {
			continue
		}
		matchedTokens := candidateMatchedTokens[docID]
		exTokens := 0
		if docID < len(idx.exTokenCount) {
			exTokens = idx.exTokenCount[docID]
		}

		if exTokens >= 2 {
			if matchedTokens < 2 {
				continue
			}
		} else {
			// For 1-token examples, require high IDF (exTotal >= singleTokenMinIDF)
			if exTotal < singleTokenMinIDF {
				continue
			}
		}

		cov := min(1.0, interIDF/exTotal)
		if exTokens < 2 {
			if cov < singleTokenCoverageThreshold {
				continue
			}
		} else {
			if cov < defaultMatchCoverageThreshold {
				continue
			}
		}
		tfFactor := candidateTFScore[docID] / max(1e-6, interIDF)
		passed = append(passed, candidateScore{
			docID:    docID,
			coverage: cov,
			tfWeight: cov * tfFactor,
		})
	}
	if len(passed) == 0 {
		return nil
	}

	// 3. Aggregate Top-K samples weighted by Coverage to eliminate label pollution.
	sort.Slice(passed, func(i, j int) bool {
		if passed[i].coverage != passed[j].coverage {
			return passed[i].coverage > passed[j].coverage
		}
		return passed[i].docID < passed[j].docID
	})
	topK := min(defaultTopK, len(passed))

	tagWeightedCounts := make(map[string]float64)
	var totalWeightSum float64

	for i := 0; i < topK; i++ {
		weight := passed[i].tfWeight
		cov := passed[i].coverage
		ex := idx.examples[passed[i].docID]
		totalWeightSum += cov
		tagCount := float64(max(1, len(ex.Tags)))
		for _, t := range ex.Tags {
			tagWeightedCounts[t] += weight / tagCount
		}
	}

	if len(tagWeightedCounts) == 0 || totalWeightSum <= 0 {
		return nil
	}

	// 4. Score calculation: Rank-decay gradient model ensuring healthy [8/9, 7, 6, 5, 4] distribution.
	// topK acts as a normalization denominator for average coverage across top matched slots.
	avgCov := totalWeightSum / float64(topK)
	type tagCandidate struct {
		name      string
		weightedC float64
		lift      float64
		combined  float64
	}
	var candidates []tagCandidate
	var maxWeightedC float64
	for t, weightedC := range tagWeightedCounts {
		if weightedC > maxWeightedC {
			maxWeightedC = weightedC
		}
		bg := idx.allTags[t]
		if bg <= 0 {
			bg = 0.0001
		}
		pMatch := weightedC / totalWeightSum
		lift := pMatch / bg
		combined := weightedC * math.Sqrt(max(0.1, lift))
		candidates = append(candidates, tagCandidate{
			name:      t,
			weightedC: weightedC,
			lift:      lift,
			combined:  combined,
		})
	}
	if maxWeightedC <= 0 {
		maxWeightedC = 1.0
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].combined != candidates[j].combined {
			return candidates[i].combined > candidates[j].combined
		}
		if candidates[i].weightedC != candidates[j].weightedC {
			return candidates[i].weightedC > candidates[j].weightedC
		}
		return candidates[i].name < candidates[j].name
	})

	type tagScore struct {
		name  string
		score int
	}
	var scored []tagScore
	effectiveRank := 0
	covFactor := min(1.0, math.Sqrt(max(0.0, avgCov)))
	for i, c := range candidates {
		if i > 0 {
			prev := candidates[i-1].combined
			if prev > 0 && (prev-c.combined)/prev > rankDecayTierDropRelativeThreshold {
				effectiveRank++
			}
		}
		rankBase := max(rankDecayBaseMin, rankDecayBaseMax-rankDecayStep*float64(effectiveRank))
		relWeight := c.weightedC / maxWeightedC
		raw := rankBase * covFactor * math.Pow(relWeight, rankDecayWeightPower)
		// Upper bounded by 10 as a defensive clamp; maximum expected rank score is 9.
		s := min(10, max(1, roundInt(raw)))
		scored = append(scored, tagScore{name: c.name, score: s})
	}

	if len(scored) == 0 {
		return nil
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].name < scored[j].name
	})
	if len(scored) > topN {
		scored = scored[:topN]
	}

	tagWeights := make(OrderedTagWeights, len(scored))
	matchedTags := make([]string, 0, len(scored))
	for _, ts := range scored {
		tagWeights[ts.name] = ts.score
		matchedTags = append(matchedTags, ts.name)
	}

	chunk[common.TAG_FLD] = tagWeights
	chunk["tag_kwd"] = matchedTags

	return &schema.TaggedChunk{
		Content:    text,
		Tags:       matchedTags,
		TagWeights: tagWeights,
	}
}

// ----------------------------------------------------------------------
// Cache & scheduling pipeline with language passthrough and few-shot fallback.
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

func isHighConfidenceMatch(matched *schema.TaggedChunk) bool {
	if matched == nil {
		return false
	}
	for _, score := range matched.TagWeights {
		if score >= 6 {
			return true
		}
	}
	return false
}

func (c *ExtractorComponent) runAutoTags(ctx context.Context, db *gorm.DB, in extractorInputs) ([]map[string]any, error) {
	lang := in.lang
	if lang == "" {
		lang = detectTextLanguage(in.chunks)
	}

	indexed, ok := c.resolveTagSource(ctx, lang)
	if !ok || len(in.chunks) == 0 {
		common.Info("extractor tags: skipped",
			zap.Int("chunk_count", len(in.chunks)),
			zap.Bool("has_tag_source", ok),
			zap.String("llm_id", in.llmID),
		)
		return in.chunks, nil
	}

	topN := c.Param.Tags.TopN
	tok := tokenizer.New(lang)

	var examples []schema.TaggedChunk
	var docsToTag []map[string]any
	for _, d := range in.chunks {
		if ctx.Err() != nil {
			break
		}
		matched := matchAndTagChunk(d, indexed, tok, topN)
		if matched != nil {
			if isHighConfidenceMatch(matched) {
				examples = append(examples, *matched)
			}
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
		return nil, false // Guard against caching nil index in LRU.
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
// first and second non-empty cells. Single-cell rows are accumulated as body text
// and prepended to the next tagged row, matching CSV parsing behavior.
// The second cell holds comma-separated tags.
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
		content := ""
		appendLine := func(s string) {
			if content == "" {
				content = s
			} else {
				content += "\n" + s
			}
		}
		for _, row := range rows {
			var cells []string
			for _, c := range row {
				if c = strings.TrimSpace(c); c != "" {
					cells = append(cells, c)
				}
			}
			if len(cells) == 0 {
				continue
			}
			if len(cells) == 1 {
				appendLine(cells[0])
				continue
			}
			appendLine(cells[0])
			tags := splitAndTrim(cells[1], ",")
			result = append(result, schema.TagLabel{Content: content, Tags: tags})
			content = ""
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
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Try quote-aware parse for comma
		rComma := csv.NewReader(strings.NewReader(line))
		rComma.Comma = ','
		if rec, err := rComma.Read(); err == nil {
			if len(rec) == 2 {
				comma++
			}
		} else if len(strings.Split(line, ",")) == 2 {
			comma++
		}

		// Try quote-aware parse for tab
		rTab := csv.NewReader(strings.NewReader(line))
		rTab.Comma = '\t'
		if rec, err := rTab.Read(); err == nil {
			if len(rec) == 2 {
				tab++
			}
		} else if len(strings.Split(line, "\t")) == 2 {
			tab++
		}
	}
	if scanner.Err() != nil {
		common.Warn(fmt.Sprintf("extractor tags: delimiter scan: %v", scanner.Err()))
	}
	if tab > 0 && tab >= comma {
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

func isAlphaExt(ext string) bool {
	if len(ext) < 2 || ext[0] != '.' {
		return false
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func cleanTitle(title string) string {
	title = strings.TrimSpace(title)
	if ext := filepath.Ext(title); ext != "" && len(ext) <= 6 && isAlphaExt(ext) {
		title = strings.TrimSuffix(title, ext)
	}
	return strings.TrimSpace(title)
}

func getChunkText(chunk map[string]any) string {
	if chunk == nil {
		return ""
	}

	var parts []string

	// 1. Extract main chunk content (prioritize content_with_weight, then text)
	if v, ok := chunk["content_with_weight"].(string); ok && strings.TrimSpace(v) != "" {
		parts = append(parts, strings.TrimSpace(v))
	} else if v, ok := chunk["text"].(string); ok && strings.TrimSpace(v) != "" {
		parts = append(parts, strings.TrimSpace(v))
	}

	// 2. Extract important keywords if present (supports []string, []any, string)
	if kwdRaw, exists := chunk["important_kwd"]; exists && kwdRaw != nil {
		switch kwds := kwdRaw.(type) {
		case []string:
			for _, k := range kwds {
				if k = strings.TrimSpace(k); k != "" {
					parts = append(parts, k)
				}
			}
		case []any:
			for _, item := range kwds {
				if s, ok := item.(string); ok {
					if s = strings.TrimSpace(s); s != "" {
						parts = append(parts, s)
					}
				}
			}
		case string:
			if k := strings.TrimSpace(kwds); k != "" {
				parts = append(parts, k)
			}
		}
	}

	// 3. Fallback: only extract title/name if chunk has NO text content or keywords
	if len(parts) == 0 {
		for _, titleKey := range []string{"docnm_kwd", "title_tks", "title"} {
			if v, ok := chunk[titleKey].(string); ok && strings.TrimSpace(v) != "" {
				if cleaned := cleanTitle(v); cleaned != "" {
					parts = append(parts, cleaned)
					break
				}
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

// detectTextLanguage detects language from chunk samples across the document.
// Priority order:
// 1. Japanese Kana (\u3040-\u309F, \u30A0-\u30FF) -> "Japanese" (Kana is exclusively Japanese)
// 2. Korean Hangul (\uAC00-\uD7AF, \u1100-\u11FF) -> "Korean" (Hangul is exclusively Korean)
// 3. CJK Unified Ideographs (\u4E00-\u9FFF, \u3400-\u4DBF) -> "Chinese"
// 4. Default -> "English"
func detectTextLanguage(chunks []map[string]any) string {
	if len(chunks) == 0 {
		return "English"
	}
	sampleIndices := make([]int, 0, 10)
	if len(chunks) <= 10 {
		for i := range chunks {
			sampleIndices = append(sampleIndices, i)
		}
	} else {
		step := float64(len(chunks)-1) / 9.0
		for i := 0; i < 10; i++ {
			idx := int(float64(i) * step)
			sampleIndices = append(sampleIndices, idx)
		}
	}

	kanaCount := 0
	hangulCount := 0
	cjkCount := 0

	for _, idx := range sampleIndices {
		if idx >= len(chunks) {
			continue
		}
		txt := getChunkText(chunks[idx])
		if txt == "" {
			continue
		}
		runes := []rune(txt)
		if len(runes) > 1000 {
			runes = runes[:1000]
		}
		for _, r := range runes {
			if (r >= 0x3040 && r <= 0x309F) || (r >= 0x30A0 && r <= 0x30FF) {
				kanaCount++
			} else if (r >= 0xAC00 && r <= 0xD7AF) || (r >= 0x1100 && r <= 0x11FF) {
				hangulCount++
			} else if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) {
				cjkCount++
			}
		}
	}

	if kanaCount > 0 {
		return "Japanese"
	}
	if hangulCount > 0 {
		return "Korean"
	}
	if cjkCount > 0 {
		return "Chinese"
	}
	return "English"
}

func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3400 && r <= 0x4dbf) {
			return true
		}
	}
	return false
}

func roundInt(f float64) int {
	if f < 0 {
		return int(f - 0.5)
	}
	return int(f + 0.5)
}

// ----------------------------------------------------------------------
// Phase 2 LLM tagging with real example cold-start and dot sanitization
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

	textHash := int64(xxhash.Sum64String(text))
	var picked []schema.TaggedChunk
	if len(examples) > 2 {
		picked = sampleWithoutReplacement(examples, 2, textHash)
	} else if len(examples) > 0 {
		picked = examples
	} else if idx != nil && len(idx.examples) > 0 {
		// Cold-start fallback: sample real examples deterministically using text hash seed
		sampleCount := min(2, len(idx.examples))
		r := rand.New(rand.NewPCG(uint64(textHash), uint64(textHash^0x5bf03635e0689dd2)))
		perm := r.Perm(len(idx.examples))[:sampleCount]
		for _, idxDoc := range perm {
			ex := idx.examples[idxDoc]
			weights := make(OrderedTagWeights, len(ex.Tags))
			for tagRank, t := range ex.Tags {
				score := max(3, 9-tagRank*3)
				weights[t] = score
			}
			picked = append(picked, schema.TaggedChunk{
				Content:    ex.Content,
				Tags:       ex.Tags,
				TagWeights: weights,
			})
		}
	}

	if cached := getTaggerLLMCache(ctx, llmID, text, allTags, picked, topN); cached != nil {
		chunk[common.TAG_FLD] = cached
		chunk["tag_kwd"] = sortedTagWeightsKeys(cached)
		return
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
	var result OrderedTagWeights
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
		chunk["tag_kwd"] = sortedTagWeightsKeys(result)
		setTaggerLLMCache(ctx, llmID, text, allTags, picked, topN, result)
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

func parseTaggerResponse(raw string, topN int) OrderedTagWeights {
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

	result := make(OrderedTagWeights, len(obj))
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
			if existing, ok := result[cleanKey]; !ok || score > existing {
				result[cleanKey] = score
			}
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
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].v != sorted[j].v {
				return sorted[i].v > sorted[j].v
			}
			return sorted[i].k < sorted[j].k
		})
		result = make(OrderedTagWeights, topN)
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

func taggerCacheKey(llmID, text string, allTags map[string]float64, examples []schema.TaggedChunk, topN int) string {
	hasher := xxhash.New()
	hasher.Write([]byte(llmID))
	hasher.Write([]byte("\x00"))
	hasher.Write([]byte(text))
	hasher.Write([]byte("\x00"))
	tagNames := sortedTagNames(allTags)
	hasher.Write([]byte(strings.Join(tagNames, ",")))
	hasher.Write([]byte("\x00"))
	for _, ex := range examples {
		hasher.Write([]byte(ex.Content))
		hasher.Write([]byte("\x00"))
		tagsJSON, _ := json.Marshal(ex.TagWeights)
		hasher.Write(tagsJSON)
		hasher.Write([]byte("\x00"))
	}
	hasher.Write([]byte(fmt.Sprintf("%d", topN)))
	return fmt.Sprintf("tagger:%x", hasher.Sum64())
}

func getTaggerLLMCache(ctx context.Context, llmID, text string, allTags map[string]float64, examples []schema.TaggedChunk, topN int) map[string]int {
	client := redis.Get()
	if client == nil {
		return nil
	}
	key := taggerCacheKey(llmID, text, allTags, examples, topN)
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

func setTaggerLLMCache(ctx context.Context, llmID, text string, allTags map[string]float64, examples []schema.TaggedChunk, topN int, result map[string]int) {
	if result == nil {
		return
	}
	client := redis.Get()
	if client == nil {
		return
	}
	key := taggerCacheKey(llmID, text, allTags, examples, topN)
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

func sortedTagWeightsKeys(m map[string]int) []string {
	if len(m) == 0 {
		return nil
	}
	type kv struct {
		k string
		v int
	}
	kvs := make([]kv, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].v != kvs[j].v {
			return kvs[i].v > kvs[j].v
		}
		return kvs[i].k < kvs[j].k
	})
	keys := make([]string, len(kvs))
	for i, item := range kvs {
		keys[i] = item.k
	}
	return keys
}

func sampleWithoutReplacement(slice []schema.TaggedChunk, k int, seed int64) []schema.TaggedChunk {
	if len(slice) == 0 || k <= 0 {
		return nil
	}
	if len(slice) <= k {
		out := make([]schema.TaggedChunk, len(slice))
		copy(out, slice)
		return out
	}
	r := rand.New(rand.NewPCG(uint64(seed), uint64(seed^0x5bf03635e0689dd2)))
	perm := r.Perm(len(slice))
	out := make([]schema.TaggedChunk, k)
	for i := 0; i < k; i++ {
		out[i] = slice[perm[i]]
	}
	return out
}
