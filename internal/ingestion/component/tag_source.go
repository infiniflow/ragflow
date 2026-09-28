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

// This file owns the tag source file corpus: everything that turns the
// parser_config tag_file_id (or raw bytes) into tag examples, an inverted
// index, or a tag vocabulary. Both consumers live here on purpose - ingestion
// time tagging (ExtractorComponent) and retrieval time label question
// (tag_query.go) read the same files through the same caches, so the
// resolution, ownership check, parsing entry point, index build and caches
// must not drift apart. The low-level xlsx/csv/txt readers this file calls
// still live in extractor_tag.go.
//
// Sharing the caches is not sharing one index: the key carries the analyzer
// language, and ingestion (dataset language) differs from retrieval
// (tagQueryLang), so each builds its own. That is required - an index must be
// tokenized with the analyzer its own caller matches against.

package component

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

// bgSmoothing is the Dirichlet smoothing constant for background prior tag probabilities.
const bgSmoothing = 10.0

// MemoryTagIndex is an in-memory inverted index of tag examples (immutable after construction, safe for concurrent use).
//
// It is built by buildMemoryTagIndex and loaded through loadOrBuildTagFileIndex.
// The retrieval-side readers of an index (TagProportions, MatchTagQuery) live in
// tag_query.go, next to the loading path that serves retrieval.
type MemoryTagIndex struct {
	examples     []schema.TagLabel
	postings     map[string][]int   // word -> doc_ids (compact slice without per-doc map overhead)
	idfs         map[string]float64 // word -> idf
	exTotalIDF   []float64          // doc_id -> sum(IDF(w))
	exTokenCount []int              // doc_id -> number of unique tokens
	allTags      map[string]float64 // tag -> background prob (S=10)

	// tok is the analyzer the index was tokenized with. Retrieval must match
	// the question with the same analyzer, so the index carries it instead of
	// trusting every caller to pass an equal one: a mismatch produces no error,
	// just an index that silently never matches. Tokenizer is a stateless value
	// (it only holds the language; the analyzer comes from a pool per call), so
	// it is safe to keep and share on this immutable index.
	tok tokenizer.Tokenizer

	// tagCounts/tagTotal are the raw occurrence counts behind allTags. They are
	// index constants, so TagProportions can derive any smoothing S from them
	// without walking the examples again on every query.
	tagCounts map[string]int
	tagTotal  int
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
		tok:          tok,
		tagCounts:    allTagCounts,
		tagTotal:     totalTagCount,
	}
}

// ----------------------------------------------------------------------
// Index caches: the built index is keyed by the tag file's size/update time,
// so an edited tag file is re-read instead of being served stale.
// ----------------------------------------------------------------------

const tagSourceCacheMax = 128

type boundedTagCache struct {
	mu     sync.Mutex
	cap    int
	items  map[string]*MemoryTagIndex
	recent []string
}

// newBoundedTagCache returns an LRU-bounded cache. A non-positive cap disables
// caching: nothing is retained, which is what store() would do anyway after
// evicting the entry it just inserted, and it keeps the eviction loop bounded.
func newBoundedTagCache(cap int) *boundedTagCache {
	return &boundedTagCache{
		cap:   cap,
		items: make(map[string]*MemoryTagIndex, max(cap, 0)),
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
	// cap <= 0 disables caching, and a nil index must never be cached: load()
	// reports a stored key as a hit, so caching nil would hand callers a nil
	// *MemoryTagIndex that looks valid.
	if c.cap <= 0 || val == nil {
		return val
	}
	c.items[key] = val
	c.recent = append(c.recent, key)
	c.evictLocked()
	return val
}

// evictLocked drops the least recently used entries until the cache fits its
// cap. Every pass pops one element, so it always terminates.
func (c *boundedTagCache) evictLocked() {
	for len(c.items) > c.cap {
		if len(c.recent) == 0 {
			// items and recent have drifted apart (only store appends to both,
			// so this needs a bug to happen): recent[0] would panic, and no
			// tracked key is left to evict, so reset instead of staying over
			// the cap forever - which would wedge the cache with entries it can
			// never evict again.
			c.items = make(map[string]*MemoryTagIndex, max(c.cap, 0))
			return
		}
		oldest := c.recent[0]
		c.recent = c.recent[1:]
		delete(c.items, oldest)
	}
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

// loadOrBuildTagFileIndex loads one tag source file's records as an in-memory
// index, reusing the shared per-file index cache. It is the single loading path
// for both ingestion (ExtractorComponent.loadTagFileIndexed) and retrieval
// (LoadTagQueryIndex), so the two cannot drift into different resolution,
// ownership checks or parse entry points.
//
// The cache key carries lang (the analyzer language), so the two consumers
// share one index only when they ask for the same language. Keeping lang in
// the key is what keeps an index tokenized with the analyzer its caller
// matches against.
//
// The file row must already be resolved inside the caller's tenant.
// It returns (nil, nil) when the file yields no usable records.
func loadOrBuildTagFileIndex(ctx context.Context, f *entity.File, tenantID, lang string) (*MemoryTagIndex, bool, error) {
	cacheKey := tagSourceFileCacheKey(f, lang)
	if cached, ok := tagSourceFileIndexCache.load(cacheKey); ok {
		return cached, true, nil // reused: no bytes read.
	}
	data, err := fetchTagSourceBytes(ctx, f, tenantID)
	if err != nil {
		return nil, false, err
	}
	idx, ok := buildIndexedTagSourceFromBytes(data, f.Name, lang)
	if !ok || idx == nil {
		return nil, false, nil // Guard against caching nil index in LRU.
	}
	return tagSourceFileIndexCache.store(cacheKey, idx), false, nil
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
		return parseCSVQuoteAwareReader(bytes.NewReader(data), csvTagSourceDelimiter(data)), nil
	case ".txt":
		delimiter := detectCSVDelimiterBytes(data)
		return parseCSVTagSourceReader(bytes.NewReader(data), delimiter), nil
	default:
		return nil, fmt.Errorf("unsupported tag source extension %q: only .xlsx, .txt and .csv are supported", filepath.Ext(filename))
	}
}

// ----------------------------------------------------------------------
// Tag source file readers, mirroring rag/app/tag.py's chunk(): xlsx is read
// per-sheet, .csv with a quote-aware reader, .txt with the delimiter-detecting
// one. Only parseTagSourceByFilename above calls into this section.
// ----------------------------------------------------------------------

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

// csvTagSourceDelimiter mirrors rag/app/tag.py's .csv path: the delimiter is
// TAB as soon as the file contains one, comma otherwise. Python's docstring
// asks for TAB-separated .csv, so the comma default must not be assumed.
func csvTagSourceDelimiter(data []byte) rune {
	if bytes.Contains(data, []byte("\t")) {
		return '\t'
	}
	return ','
}

// parseCSVQuoteAwareReader mirrors rag/app/tag.py's .csv path: the whole text
// goes through a single encoding/csv reader, so a quoted field may contain the
// delimiter or span several physical lines (Python's csv.reader pulls further
// lines from its generator while inside quotes; Go's reader does the same).
// Records that do not yield exactly two non-empty columns are accumulated as
// body text using the raw lines they span, and the second column holds
// comma-separated tags.
//
// LazyQuotes mirrors Python's tolerance for stray quote characters: a bare "
// inside an unquoted field is data there, not a fatal parse error.
func parseCSVQuoteAwareReader(r io.Reader, delimiter rune) []schema.TagLabel {
	data, err := io.ReadAll(r)
	if err != nil {
		common.Warn(fmt.Sprintf("extractor tags: read csv tag source: %v", err))
		return nil
	}
	text := string(data)

	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1 // Python's csv.reader accepts variable column counts.
	reader.LazyQuotes = true

	result := make([]schema.TagLabel, 0)
	content := ""
	appendLine := func(s string) {
		if content == "" {
			content = s
		} else {
			content += "\n" + s
		}
	}

	// InputOffset() reports where a record ended, so the text between the
	// previous offset and this one is the raw span the record was parsed from,
	// the equivalent of Python's lines[prev_line_num : reader.line_num].
	start := 0
	for {
		rec, err := reader.Read()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				common.Warn(fmt.Sprintf("extractor tags: parse csv tag source: %v", err))
			}
			break
		}
		end := int(reader.InputOffset())
		raw := strings.TrimSuffix(strings.TrimSuffix(text[start:end], "\n"), "\r")
		start = end
		// encoding/csv skips blank lines without reporting them, while Python's
		// generator hands them to the reader as empty records, which land in
		// the body text. Replay them as empty lines.
		for strings.HasPrefix(raw, "\n") || strings.HasPrefix(raw, "\r\n") {
			if raw[0] == '\r' {
				raw = raw[1:]
			}
			raw = raw[1:]
			appendLine("")
		}
		row := stripEmptyFields(rec)
		if len(row) != 2 {
			appendLine(raw)
			continue
		}
		appendLine(row[0])
		tags := splitAndTrim(row[1], ",")
		result = append(result, schema.TagLabel{Content: content, Tags: tags})
		content = ""
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

// TagVocabularyFromBytes is the storage/DAO-free core of
// TagVocabularyFromTagFileID. It parses a tag source file's raw bytes (mirroring
// rag/app/tag.py's chunk format) and returns the vocabulary as tag -> number of
// source examples that mention the tag; a tag repeated within one source example
// counts once, matching buildMemoryTagIndex's per-sample deduplication.
func TagVocabularyFromBytes(data []byte, filename string) (map[string]int, error) {
	labels, err := parseTagSourceByFilename(data, filename)
	if err != nil {
		return nil, err
	}
	return tagVocabularyFromLabels(labels), nil
}

// tagVocabularyFromLabels counts, per tag, how many source examples mention it.
// A tag repeated within one example counts once.
func tagVocabularyFromLabels(labels []schema.TagLabel) map[string]int {
	counts := make(map[string]int)
	for _, lbl := range labels {
		seen := make(map[string]struct{}, len(lbl.Tags))
		for _, t := range lbl.Tags {
			t = strings.TrimSpace(strings.ReplaceAll(t, ".", "_"))
			if t != "" {
				if _, exists := seen[t]; exists {
					continue
				}
				seen[t] = struct{}{}
				counts[t]++
			}
		}
	}
	return counts
}

// readTagSourceFile resolves a tag source file id (parser_config.tags.tag_file_id)
// inside ownerTenantID and returns the file row together with its raw bytes.
//
// ownerTenantID is the tenant of the dataset that configured the file and is
// required: tag_file_id is user-writable through parser_config, so the file must
// be proven to belong to that tenant before its bytes are read (IDOR, CWE-639).
// An empty ownerTenantID fails closed. Note that storage.Get's tenant argument
// does not establish file ownership by itself.
func readTagSourceFile(ctx context.Context, tagFileID, ownerTenantID string) (*entity.File, []byte, error) {
	f, err := resolveTagSourceFile(ctx, tagFileID, ownerTenantID)
	if err != nil {
		return nil, nil, err
	}
	data, err := fetchTagSourceBytes(ctx, f, ownerTenantID)
	if err != nil {
		return nil, nil, err
	}
	return f, data, nil
}

const (
	// tagSourceFileRowTTL is how long a resolved tag source file row is reused
	// before the DB is asked again. The row only proves ownership and supplies
	// the index cache key (size/update time), and retrieval labels a question
	// once per search leg, so a short TTL removes that per-leg query while
	// keeping an edited tag file from being served stale for long.
	tagSourceFileRowTTL = 30 * time.Second

	// tagSourceFileRowCacheMax bounds the number of cached file rows.
	tagSourceFileRowCacheMax = 512
)

// tagSourceFileRowCache is a TTL-bounded cache of resolved tag source file
// rows, keyed by (file id, tenant). Only positive lookups are cached: an
// unresolvable tag_file_id keeps failing closed instead of being pinned.
type tagSourceFileRowCache struct {
	mu    sync.Mutex
	items map[string]tagSourceFileRow
	order []string
}

type tagSourceFileRow struct {
	file   *entity.File
	expiry time.Time
}

func (c *tagSourceFileRowCache) load(key string, now time.Time) (*entity.File, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	row, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if now.After(row.expiry) {
		delete(c.items, key)
		return nil, false
	}
	return row.file, true
}

// store caches a copy of the row and returns another copy, so callers cannot
// mutate what later lookups will hand out.
func (c *tagSourceFileRowCache) store(key string, f *entity.File, now time.Time) *entity.File {
	cached := *f
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; !ok {
		for len(c.items) >= tagSourceFileRowCacheMax && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.items, oldest)
		}
		c.order = append(c.order, key)
	}
	c.items[key] = tagSourceFileRow{file: &cached, expiry: now.Add(tagSourceFileRowTTL)}
	out := *f
	return &out
}

var tagSourceFileRows = &tagSourceFileRowCache{items: make(map[string]tagSourceFileRow, 64)}

// errTagSourceNotFound marks a failure to resolve one tag source file id
// against its owning tenant. It deliberately covers only the resolution step —
// no row, a row without a storage location — and NOT the storage step: a
// missing backend or a failed object read says nothing about this particular
// id and must stay a hard failure.
var errTagSourceNotFound = errors.New("tag source file not found")

// IsTagSourceNotFound reports whether err means this specific tag source file
// id cannot be resolved, as opposed to a transient database failure or the
// storage layer being unavailable. Callers that treat a source as optional —
// one inherited from a document, where a stale reference is expected once the
// file has been deleted — may skip it; a source the dataset configured
// explicitly, and any database or storage failure, must still fail loudly.
func IsTagSourceNotFound(err error) bool {
	return errors.Is(err, errTagSourceNotFound)
}

// resolveTagSourceFile looks up a tag source file inside ownerTenantID and
// validates that it is a readable stored object. Resolved rows are reused for
// tagSourceFileRowTTL so repeated lookups of the same file do not each cost a
// query.
func resolveTagSourceFile(ctx context.Context, tagFileID, ownerTenantID string) (*entity.File, error) {
	key := tagFileID + "\x00" + ownerTenantID
	now := time.Now()
	if cached, ok := tagSourceFileRows.load(key, now); ok {
		return cached, nil
	}
	f, err := dao.NewFileDAO().GetByIDAndTenant(ctx, dao.DB, tagFileID, ownerTenantID)
	if err != nil {
		// The DAO returns database errors unchanged, and only a genuine miss
		// means this id is dead: wrapping a transient failure as well would
		// let a caller skip it and report an incomplete vocabulary as success.
		if dao.IsNotFoundErr(err) {
			return nil, fmt.Errorf("tag source file %q not found: %w: %w", tagFileID, errTagSourceNotFound, err)
		}
		return nil, fmt.Errorf("resolve tag source file %q: %w", tagFileID, err)
	}
	if f == nil {
		return nil, fmt.Errorf("tag source file %q not found in tenant %q: %w", tagFileID, ownerTenantID, errTagSourceNotFound)
	}
	if f.Location == nil || *f.Location == "" {
		return nil, fmt.Errorf("tag source file %q has no storage location: %w", tagFileID, errTagSourceNotFound)
	}
	common.Info(fmt.Sprintf("tag source: file_id=%q name=%q parent_id=%q location=%q",
		tagFileID, f.Name, f.ParentID, *f.Location))
	return tagSourceFileRows.store(key, f, now), nil
}

// fetchTagSourceBytes reads a resolved tag source file from the storage backend.
func fetchTagSourceBytes(ctx context.Context, f *entity.File, ownerTenantID string) ([]byte, error) {
	stg := resolveStorage()
	if stg == nil {
		return nil, fmt.Errorf("tag source file %q: no storage backend registered", f.ID)
	}
	data, err := stg.Get(ctx, f.ParentID, *f.Location, ownerTenantID)
	if err != nil {
		return nil, fmt.Errorf("load tag source file %q/%q: %w", f.ParentID, *f.Location, err)
	}
	return data, nil
}

// TagVocabularyFromTagFileID loads a tag source file (parser_config.tags.tag_file_id)
// and returns the tag vocabulary it defines. It is the Go-native source of the
// selectable-tag list surfaced by the tag-options/aggregation API.
//
// It returns (nil, nil) when tagFileID is empty.
func TagVocabularyFromTagFileID(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error) {
	if tagFileID == "" {
		return nil, nil
	}
	common.Info(fmt.Sprintf("tag_vocab: loading tag source file_id=%q", tagFileID))
	f, data, err := readTagSourceFile(ctx, tagFileID, ownerTenantID)
	if err != nil {
		return nil, err
	}
	common.Info(fmt.Sprintf("tag_vocab: file_id=%q loaded %d bytes", tagFileID, len(data)))
	return TagVocabularyFromBytes(data, f.Name)
}
