// Command covsweep answers one question for a whole document set:
//
//	does the search index still serve every passage of every source document?
//
// It exists because "document.chunk_num equals the number of chunk documents in ES" only proves
// the two counters agree - it cannot see a document whose chunks silently dropped a passage. The
// Python scripts under scripts/ answer the same question for a handful of documents; this tool
// answers it for a whole document set in one pass, fast enough to run after every repair, which is
// what makes the answer useful.
//
// The document set is never assumed: --docs, --corpus, --index and --kb have no built-in defaults,
// so a run either names its inputs or stops before it reads anything.
//
// Method, per document:
//
//  1. pull every chunk text of the document out of ES (kb_id + docnm_kwd filter) and join them;
//  2. project both the source file and the joined chunk text onto their alphanumeric characters
//     (lowercased), remembering for every file character which byte of the original it came from;
//  3. match 16-character k-grams between the two projections with a rolling hash; a k-gram the
//     chunk text contains covers the file bytes it came from. The projection is what makes this
//     honest: the ingest strips punctuation AND closes the gap it leaves, so a source line
//     "* China at the Asian Games * China at the Paralympics" is served as "...GamesChina at the
//     Paralympics" - comparing bytes or word tokens marks that passage missing, which is how an
//     earlier version of this tool reported a uniform ~78% coverage with no document above 90%;
//  4. report a run of uncovered file bytes as a missing passage only when it is at least --min-gap
//     bytes long, which is far longer than the remainder of a re-punctuated boundary.
//
// --method window keeps the raw byte-window comparison for the record, where the false-alarm rate
// on markup-heavy documents can be seen.
//
// Verdict semantics: a word counts as SERVED only when the chunk text carries it as its own token.
// Gluing is not tolerated - an entry whose words were run together ("ChinaChina", "HathiTrustThe")
// cannot be matched by a query for "China", so those words are defects even though their characters
// are present. The report splits them into "content never arrived" and "glued into a longer token"
// so the two failure modes can be told apart, but both land in the flagged list:
//
//	<out>/coverage_go.jsonl  one row per document (word_pct, missing_strict_words, absent/fused splits, gaps)
//	<out>/below99.txt        documents with any unserved word - strict, includes glued ones
//	<out>/fused.txt          the glued subset, for measuring a separator fix
//
// Concurrency, and where the time actually goes (the tool prints a phase breakdown on exit):
//
//	fetchers   --shards goroutines, each paging its own shard of the document set with
//	           search_after. A page is megabytes of content_with_weight, so each shard also runs a
//	           decoder goroutine: the fetch asks for page N+1 while the decode parses page N,
//	           otherwise the socket idles for the whole parse;
//	workers    --workers goroutines doing tokenising and hashing;
//	writer     one goroutine appending JSONL and printing progress.
//
// A document is emitted as soon as the chunk texts seen for it reach the chunk_num column of
// --docs, so one slow shard cannot stall the rest; the flip side is that a stale chunk_num either
// truncates a document or emits it with nothing at all (see Usage).
//
// The phase breakdown exists because this workload does not scale the way "add goroutines"
// suggests: the index has few shards, so the fetch half saturates before the CPU does, and the
// compute half is dominated by the largest documents rather than by their number - measuring beats
// guessing.
//
// Usage:
//
//	# "<doc id>\t<name>\t<chunk_num>" rows, one per document, with one source file per row's name
//	# column in --corpus. Regenerate the list immediately before a run: chunk_num is the per
//	# document threshold above, so a stale value truncates a document, or at 0 emits it empty.
//	docker exec <mysql-container> sh -c 'MYSQL_PWD=<password> mysql -N -uroot <database> \
//	    -e "SELECT id,name,chunk_num FROM document WHERE kb_id=\"<kb>\";"' > /tmp/cov_docs.tsv
//
//	go run ./tools/covsweep --docs /tmp/cov_docs.tsv --corpus <sources> \
//	    --index <es-index> --kb <dataset-id> --out outputs/coverage
//	go run ./tools/covsweep ... --limit 400                    # calibrate on a subset
//	go run ./tools/covsweep ... --only-file <out>/below99.txt   # re-check only the flagged ones
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
)

type doc struct {
	ID     string
	Name   string
	Chunks int
}

type job struct {
	Name   string
	ID     string
	Chunks int
	Texts  []string
}

type gap struct {
	Start   int    `json:"start"`
	Length  int    `json:"length"`
	Context string `json:"context"`
}

type result struct {
	Name        string  `json:"name"`
	ID          string  `json:"id"`
	MySQLChunks int     `json:"mysql_chunks"`
	ChunkCount  int     `json:"chunk_count"`
	FileChars   int     `json:"file_chars"`
	FileAlnum   int     `json:"file_alnum"` // alphanumeric characters the file projects to
	CoveredChar int     `json:"covered_chars"`
	CoveragePct float64 `json:"coverage_pct"` // order-sensitive: 16-gram containment
	GapChars    int     `json:"gap_chars"`
	// WordPct is the order-insensitive answer to "is anything missing": the share of the file's
	// word occurrences (multiset) that the chunk text also carries. It is the headline number
	// because the ingest legitimately re-orders words inside tables (a source row
	// "Longlist | 2021 | Baba Badji | Ghost Letters" is served as the cells
	// "<td>2021</td><td>Baba Badji</td><td>Ghost Letters</td><td>Longlist</td>"), which the
	// order-sensitive metric reports as missing text on every table-heavy document.
	WordPct            float64 `json:"word_pct"`
	FileWords          int     `json:"file_words"`
	MissingOccurrences int     `json:"missing_occurrences"`
	MissingWords       int     `json:"missing_words"`
	// AbsentWords counts file words the chunks do not contain at all - not even inside a longer
	// token. This is the "content never arrived" breakdown of MissingStrictWords.
	AbsentWords int `json:"absent_words"`
	// FusedWords counts file words reachable ONLY inside a longer chunk token, i.e. text the ingest
	// glued together by dropping the separator between entries. Gluing is a defect, not a cosmetic
	// difference: the merged token cannot be matched (a query for "China" does not match
	// "chinachina"), so these words are counted as not served. They are broken out here only so a
	// glued document can be told apart from one whose text is missing.
	FusedWords int `json:"fused_words"`
	// MissingStrictWords is the headline defect count: file words that are not available as their own
	// token in the chunk text - absent plus fused. It does not tolerate gluing.
	MissingStrictWords int      `json:"missing_strict_words"`
	StrictSamples      []string `json:"strict_samples,omitempty"`
	AbsentSamples      []string `json:"absent_samples,omitempty"`
	FusedSamples       []string `json:"fused_samples,omitempty"`
	ComputeMs          float64  `json:"compute_ms"`
	Gaps               []gap    `json:"gaps,omitempty"`
	Error              string   `json:"error,omitempty"`
}

// stats is the phase breakdown. Every counter is atomic because the phases run in different
// goroutines; wall time is what matters for the fetch (it is one socket per shard).
type stats struct {
	fetchNanos  int64
	fetchBytes  int64
	pages       int64
	decodeNanos int64
	computeNano int64
	docs        int64
	emptyDocs   int64 // documents emitted with no chunks at all though MySQL expected some
	errs        int64
	slowest     sync.Map // name -> float64 ms
}

func (s *stats) addEmpty() { atomic.AddInt64(&s.emptyDocs, 1) }
func (s *stats) addError() { atomic.AddInt64(&s.errs, 1) }

func (s *stats) addFetch(d time.Duration, bytes int) {
	atomic.AddInt64(&s.fetchNanos, int64(d))
	atomic.AddInt64(&s.fetchBytes, int64(bytes))
	atomic.AddInt64(&s.pages, 1)
}

func (s *stats) addDecode(d time.Duration) { atomic.AddInt64(&s.decodeNanos, int64(d)) }
func (s *stats) addCompute(d time.Duration) {
	atomic.AddInt64(&s.computeNano, int64(d))
	atomic.AddInt64(&s.docs, 1)
}

func (s *stats) noteSlow(name string, ms float64) { s.slowest.Store(name, ms) }

func (s *stats) report(wall time.Duration, cpu time.Duration) {
	secs := wall.Seconds()
	fmt.Printf("\n=== phase breakdown (wall %.1fs, cpu %.1fs, %.2f cores busy) ===\n",
		secs, cpu.Seconds(), cpu.Seconds()/max(secs, 1e-9))
	fmt.Printf("  fetch  : %6.1fs wall over %d pages, %.1f MB from ES\n",
		float64(atomic.LoadInt64(&s.fetchNanos))/1e9, s.pages, float64(s.fetchBytes)/1e6)
	fmt.Printf("  decode : %6.1fs cpu (JSON parse of those pages)\n", float64(atomic.LoadInt64(&s.decodeNanos))/1e9)
	fmt.Printf("  compute: %6.1fs cpu over %d docs (tokenise+hash+compare)\n",
		float64(atomic.LoadInt64(&s.computeNano))/1e9, s.docs)
	if empty := atomic.LoadInt64(&s.emptyDocs); empty > 0 {
		fmt.Printf("  WARNING: %d documents were emitted with no chunks at all although MySQL expects some\n"+
			"           (paging lost them - check the sort key before trusting this run)\n", empty)
	}
	if errs := atomic.LoadInt64(&s.errs); errs > 0 {
		fmt.Printf("  WARNING: %d shards aborted on an error - the numbers above are incomplete\n", errs)
	}
	type kv struct {
		name string
		ms   float64
	}
	var all []kv
	s.slowest.Range(func(k, v any) bool {
		all = append(all, kv{k.(string), v.(float64)})
		return true
	})
	sort.Slice(all, func(i, j int) bool { return all[i].ms > all[j].ms })
	if len(all) > 0 {
		fmt.Printf("  slowest documents: ")
		for i := 0; i < len(all) && i < 5; i++ {
			fmt.Printf("%s(%.0fms) ", all[i].name, all[i].ms)
		}
		fmt.Println()
	}
}

var whitespaceRun = regexp.MustCompile(`\s+`)

func normalise(s string) string { return strings.TrimSpace(whitespaceRun.ReplaceAllString(s, " ")) }

// ---------------------------------------------------------------------------
// Elasticsearch

type esClient struct {
	base string
	auth string
	http *http.Client
}

type searchHits struct {
	Hits struct {
		Hits []struct {
			Sort   []any `json:"sort"`
			Source struct {
				Name    string `json:"docnm_kwd"`
				Content string `json:"content_with_weight"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func newESClient(base, user, pass string, conns int) *esClient {
	transport := &http.Transport{
		// The default transport keeps only 2 idle connections per host, which is the wrong shape
		// for a sharded reader: every shard past the second would re-dial instead of reusing.
		MaxIdleConns:        conns * 4,
		MaxIdleConnsPerHost: conns * 2,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		DialContext:         (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   false,
	}
	return &esClient{
		base: strings.TrimRight(base, "/"),
		auth: base64.StdEncoding.EncodeToString([]byte(user + ":" + pass)),
		http: &http.Client{Transport: transport, Timeout: 10 * time.Minute},
	}
}

// searchRaw posts a query and returns the response body untouched, so the caller can decode it on
// its own goroutine while the next page is already being requested.
func (c *esClient) searchRaw(index string, body map[string]any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/"+index+"/_search", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+c.auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("es %s: %s", resp.Status, truncate(string(raw), 300))
	}
	return raw, nil
}

func decodeHits(raw []byte) (*searchHits, error) {
	var out searchHits
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// produceShard walks one shard of the document set and emits each document as soon as every chunk
// MySQL expects has arrived. The shard runs two goroutines: one keeps ES busy, one parses the page
// that just arrived.
func produceShard(es *esClient, index, kb string, shard []doc, pageSize int, jobs chan<- job, st *stats, wg *sync.WaitGroup) {
	defer wg.Done()
	pending := make(map[string]*job, len(shard))
	names := make([]string, 0, len(shard))
	for _, d := range shard {
		pending[d.Name] = &job{Name: d.Name, ID: d.ID, Chunks: d.Chunks}
		names = append(names, d.Name)
	}
	body := map[string]any{
		"size":    pageSize,
		"_source": []string{"docnm_kwd", "content_with_weight"},
		"query": map[string]any{"bool": map[string]any{"filter": []any{
			map[string]any{"term": map[string]any{"kb_id": kb}},
			// A name list keeps the response to the target documents only; a name RANGE would
			// drag in every other document of the dataset that sorts between them.
			map[string]any{"terms": map[string]any{"docnm_kwd": names}},
		}}},
		// A stable sort is not optional here: search_after over sort:_doc is documented as
		// unreliable and skipped documents once the pages got small (chunks=0 for documents whose
		// chunks were never returned). Sorting by name then chunk order keeps a document's chunks
		// contiguous across pages, which is also why paging can stop as soon as a name changes.
		"sort": []any{
			map[string]any{"docnm_kwd": map[string]any{"order": "asc", "missing": "_last"}},
			map[string]any{"chunk_order_int": map[string]any{"order": "asc", "missing": "_last"}},
		},
	}

	type page struct {
		raw []byte
		err error
	}
	pages := make(chan page, 2)
	go func() {
		defer close(pages)
		var after []any
		for {
			if after != nil {
				body["search_after"] = after
			}
			start := time.Now()
			raw, err := es.searchRaw(index, body)
			st.addFetch(time.Since(start), len(raw))
			if err != nil {
				pages <- page{err: err}
				return
			}
			// Peek at the sort of the last hit to page on, then hand the raw body over: the
			// decoding itself belongs to the other goroutine.
			last, count, perr := lastSortAndCount(raw)
			if perr != nil {
				pages <- page{err: perr}
				return
			}
			pages <- page{raw: raw}
			if count == 0 || count < pageSize {
				return
			}
			after = last
		}
	}()

	for p := range pages {
		if p.err != nil {
			fmt.Fprintf(os.Stderr, "\nFATAL shard fetch failed: %v\n", p.err)
			st.addError()
			break
		}
		start := time.Now()
		resp, err := decodeHits(p.raw)
		st.addDecode(time.Since(start))
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nFATAL shard decode failed (response shape changed?): %v\n", err)
			st.addError()
			break
		}
		for _, hit := range resp.Hits.Hits {
			if j, ok := pending[hit.Source.Name]; ok {
				j.Texts = append(j.Texts, hit.Source.Content)
			}
		}
		for name, j := range pending {
			if len(j.Texts) >= j.Chunks {
				if len(j.Texts) == 0 && j.Chunks > 0 {
					st.addEmpty()
				}
				jobs <- *j
				delete(pending, name)
			}
		}
	}
	for _, j := range pending {
		if len(j.Texts) == 0 && j.Chunks > 0 {
			st.addEmpty()
		}
		jobs <- *j
	}
}

// lastSortAndCount reads just enough of a search response to continue paging: the sort value of
// the last hit and how many hits came back.
func lastSortAndCount(raw []byte) ([]any, int, error) {
	var envelope struct {
		Hits struct {
			Hits []struct {
				Sort []any `json:"sort"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, 0, err
	}
	n := len(envelope.Hits.Hits)
	if n == 0 {
		return nil, 0, nil
	}
	return envelope.Hits.Hits[n-1].Sort, n, nil
}

// ---------------------------------------------------------------------------
// Coverage: character k-grams over an alphanumeric-only projection
//
// Comparing raw bytes, or even word tokens, over-reports missing text on this corpus. The ingest
// removes punctuation and closes the gap it leaves behind, so the source line
//
//	* China at the Asian Games * China at the Paralympics
//
// is served as
//
//	China at the Asian GamesChina at the Paralympics
//
// Every word is present, but "Games" and "China" are one token in the index and two in the file, so
// any token-boundary comparison calls that passage missing - which is how the first version of this
// tool managed to report a uniform ~78% coverage for 8,398 documents with no document above 90%.
//
// The fix is to compare the two sides on a projection that keeps only letters and digits, which is
// invariant under punctuation removal and word concatenation, and to match 16-character k-grams on
// it with a rolling hash. Each file character remembers where it came from, so matched k-grams still
// translate back into byte ranges of the original file - the report stays in file coordinates.

const kGram = 16

// hashBase is the FNV-1a prime; any odd multiplier works for a rolling hash.
const hashBase = 1099511628211

type projection struct {
	text    []byte  // alphanumeric-only, lowercased
	offsets []int32 // offsets[i] = byte offset in the original string of text[i]
}

// project folds a string onto its alphanumeric characters. Letters are lowercased so ASCII case
// differences in the ingest cannot register as missing content either.
func project(s string) projection {
	p := projection{text: make([]byte, 0, len(s)), offsets: make([]int32, 0, len(s))}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			p.text = append(p.text, byte(r))
		case r >= 'A' && r <= 'Z':
			p.text = append(p.text, byte(r-'A'+'a'))
		case r > unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			// Keep non-ASCII letters/digits as their rune bytes; both sides are folded the same way,
			// so equality still holds where the ingest preserved the text.
			start := len(p.text)
			for _, b := range []byte(string(r)) {
				p.text = append(p.text, b)
				p.offsets = append(p.offsets, int32(i))
				_ = start
			}
			continue
		default:
			continue
		}
		p.offsets = append(p.offsets, int32(i))
	}
	return p
}

// rollingKGramHashes hashes every k-length window of a byte slice.
func rollingKGramHashes(b []byte, k int) []uint64 {
	if len(b) < k {
		return nil
	}
	out := make([]uint64, len(b)-k+1)
	var pow uint64 = 1
	for i := 0; i < k-1; i++ {
		pow *= hashBase
	}
	var h uint64
	for i := 0; i < k; i++ {
		h = h*hashBase + uint64(b[i])
	}
	out[0] = h
	for i := k; i < len(b); i++ {
		h = (h-uint64(b[i-k])*pow)*hashBase + uint64(b[i])
		out[i-k+1] = h
	}
	return out
}

// coverageAlnum covers every file byte that takes part in a k-gram the chunk text also contains.
// Whatever is left over in runs longer than minGap bytes is a passage the index does not serve.
func coverageAlnum(fileNorm, joined string, minGap int) (covered []byte, fileAlnum int, gaps []gap) {
	fileProj := project(fileNorm)
	chunkProj := project(joined)
	fileAlnum = len(fileProj.text)
	covered = make([]byte, len(fileNorm))
	if len(fileProj.text) < kGram || len(chunkProj.text) < kGram {
		return covered, fileAlnum, nil
	}
	chunkHashes := rollingKGramHashes(chunkProj.text, kGram)
	set := make(map[uint64]struct{}, len(chunkHashes))
	for _, h := range chunkHashes {
		set[h] = struct{}{}
	}
	for i, h := range rollingKGramHashes(fileProj.text, kGram) {
		if _, ok := set[h]; !ok {
			continue
		}
		from := int(fileProj.offsets[i])
		to := int(fileProj.offsets[i+kGram-1]) + 1
		for b := from; b < to; b++ {
			covered[b] = 1
		}
	}
	// Uncovered runs, reported in file coordinates. Text the ingest merely re-punctuated shows up as
	// scattered single bytes and never reaches minGap; a dropped passage does.
	start := -1
	for i := 0; i <= len(covered); i++ {
		flag := i < len(covered) && covered[i] == 1
		switch {
		case !flag && start < 0:
			start = i
		case start >= 0 && (flag || i == len(covered)):
			// The run that began at `start` ends here: either coverage resumed, or
			// the sentinel iteration (i == len(covered), flag == false) was reached.
			// A tail the ingest never covered is a gap like any other — without this
			// arm it is discarded while its bytes still count toward GapChars.
			if i-start >= minGap {
				gaps = append(gaps, gap{Start: start, Length: i - start,
					Context: fileNorm[max(0, start-40):min(len(fileNorm), i+40)]})
			}
			start = -1
		}
	}
	return covered, fileAlnum, gaps
}

// coverageWindow is the byte-window comparison kept for the record (see the package comment): it
// reports markup rewrites as missing text.
func coverageWindow(fileNorm, joined string, window, stride, minGap int) ([]byte, []gap) {
	covered := make([]byte, len(fileNorm))
	if joined == "" || len(fileNorm) == 0 {
		return covered, nil
	}
	for pos := 0; pos < len(fileNorm); {
		end := min(pos+window, len(fileNorm))
		if strings.Contains(joined, fileNorm[pos:end]) {
			for i := pos; i < end; i++ {
				covered[i] = 1
			}
			pos += max(end-pos-stride, 1)
			continue
		}
		pos += stride
	}
	var gaps []gap
	start := -1
	for i := 0; i <= len(covered); i++ {
		flag := i < len(covered) && covered[i] == 1
		switch {
		case !flag && start < 0:
			start = i
		case start >= 0 && (flag || i == len(covered)):
			// The run that began at `start` ends here: either coverage resumed, or
			// the sentinel iteration (i == len(covered), flag == false) was reached.
			// A tail the ingest never covered is a gap like any other — without this
			// arm it is discarded while its bytes still count toward GapChars.
			if i-start >= minGap {
				gaps = append(gaps, gap{Start: start, Length: i - start,
					Context: fileNorm[max(0, start-40):min(len(fileNorm), i+40)]})
			}
			start = -1
		}
	}
	return covered, gaps
}

// tokeniseWords splits lowercased alphanumeric runs out of the text.
func tokeniseWords(s string) []string {
	out := make([]string, 0, len(s)/6+8)
	start := -1
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			out = append(out, strings.ToLower(s[start:i]))
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, strings.ToLower(s[start:]))
	}
	return out
}

type wordStats struct {
	Total       int      // word occurrences in the file
	Short       int      // distinct words whose occurrence count falls short in the chunks
	OccShort    int      // occurrences that fall short (multiset attrition)
	Absent      []string // file words the chunks do not contain at all, not even inside a longer token
	Fused       []string // file words reachable only inside a longer chunk token
	Strict      []string // Absent + Fused: words with no token of their own (sorted)
	MultisetPct float64  // share of the file's occurrences the chunks carry
}

// computeWordStats is the order-insensitive answer to two questions at once, and it deliberately
// gives gluing no quarter.
//
// A file word is SERVED only if the chunk text carries it as its own token. Two ways to fail:
//
//	Absent - the chunk text does not contain the word at all (content never arrived);
//	Fused  - the word exists only inside a longer token, because the ingest dropped a separator
//	         between entries ("GamesChina", "HathiTrustThe"). The characters are there, but the
//	         token is not: a query for "China" cannot match "chinachina", so retrieval loses the
//	         word. Counting it as served would hide a real defect behind a substring test.
//
// Strict is the union of the two - the words this document does not actually serve - and it is what
// the sweep flags on. The split exists only to tell a glued document from one whose text is gone.
//
// MultisetPct reads low (~50%) for documents whose source file repeats the same blocks twice: the
// ingest de-duplicates the repeats, so the ratio of file words to chunk words lands at ~1.98.
func computeWordStats(fileNorm, joined string) wordStats {
	stats := wordStats{}
	fileWords := map[string]int{}
	for _, w := range tokeniseWords(fileNorm) {
		fileWords[w]++
	}
	chunkWords := map[string]int{}
	for _, w := range tokeniseWords(joined) {
		chunkWords[w]++
	}
	stream := project(joined).text
	for w, n := range fileWords {
		stats.Total += n
		have := chunkWords[w]
		if have < n {
			stats.Short++
			stats.OccShort += n - have
		}
		if have > 0 {
			continue // served as its own token
		}
		if bytes.Contains(stream, []byte(w)) {
			stats.Fused = append(stats.Fused, w)
		} else {
			stats.Absent = append(stats.Absent, w)
		}
	}
	if stats.Total > 0 {
		stats.MultisetPct = 100 * float64(stats.Total-stats.OccShort) / float64(stats.Total)
	} else {
		stats.MultisetPct = 100
	}
	sort.Strings(stats.Absent)
	sort.Strings(stats.Fused)
	stats.Strict = append(append(make([]string, 0, len(stats.Absent)+len(stats.Fused)), stats.Absent...), stats.Fused...)
	sort.Strings(stats.Strict)
	return stats
}

func measure(corpusDir string, j job, opts options, st *stats) result {
	start := time.Now()
	defer func() { st.noteSlow(j.Name, float64(time.Since(start).Microseconds())/1000) }()
	res := result{Name: j.Name, ID: j.ID, MySQLChunks: j.Chunks, ChunkCount: len(j.Texts)}
	raw, err := os.ReadFile(filepath.Join(corpusDir, j.Name))
	if err != nil {
		res.Error = "FILE MISSING"
		return res
	}
	fileNorm := normalise(string(raw))
	res.FileChars = len(fileNorm)
	if fileNorm == "" {
		res.Error = "EMPTY FILE"
		return res
	}
	joined := normalise(strings.Join(j.Texts, "\n"))

	computeStart := time.Now()
	var covered []byte
	var gaps []gap
	if opts.method == "window" {
		covered, gaps = coverageWindow(fileNorm, joined, opts.window, opts.stride, opts.minGap)
	} else {
		covered, res.FileAlnum, gaps = coverageAlnum(fileNorm, joined, opts.minGap)
	}
	ws := computeWordStats(fileNorm, joined)
	res.WordPct, res.FileWords = ws.MultisetPct, ws.Total
	res.MissingWords, res.MissingOccurrences = ws.Short, ws.OccShort
	res.AbsentWords, res.FusedWords = len(ws.Absent), len(ws.Fused)
	// The strict count has to be copied here explicitly: it is the verdict the summary keys off
	// ("a word is served only when it is its own token"), and when it was left at its zero value the
	// report said "0 documents with unserved words" while listing thousands of glued ones - a gate
	// that passes because it was never wired, not because the index is clean.
	res.MissingStrictWords = len(ws.Strict)
	res.AbsentSamples, res.FusedSamples = firstN(ws.Absent, 8), firstN(ws.Fused, 8)
	res.StrictSamples = firstN(ws.Strict, 8)
	st.addCompute(time.Since(computeStart))

	count := 0
	for _, b := range covered {
		count += int(b)
	}
	res.CoveredChar = count
	res.GapChars = len(fileNorm) - count
	res.CoveragePct = float64(count) / float64(len(fileNorm)) * 100
	res.ComputeMs = float64(time.Since(start).Microseconds()) / 1000
	if len(gaps) > 5 {
		gaps = gaps[:5]
	}
	res.Gaps = gaps
	return res
}

// ---------------------------------------------------------------------------
// helpers

type options struct {
	method string
	window int
	stride int
	minGap int
}

func firstN(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func envPassword(path, key string) string {
	fh, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer fh.Close()
	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

func loadDocs(path string, limit int) ([]doc, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	var out []doc
	scanner := bufio.NewScanner(fh)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 3 || fields[0] == "" {
			continue
		}
		var n int
		fmt.Sscanf(fields[2], "%d", &n)
		out = append(out, doc{ID: fields[0], Name: fields[1], Chunks: n})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, scanner.Err()
}

func loadDone(path string) map[string]bool {
	done := map[string]bool{}
	fh, err := os.Open(path)
	if err != nil {
		return done
	}
	defer fh.Close()
	scanner := bufio.NewScanner(fh)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var r result
		if err := json.Unmarshal(scanner.Bytes(), &r); err == nil && r.Name != "" {
			done[r.Name] = true
		}
	}
	return done
}

func loadNames(path string) map[string]bool {
	if path == "" {
		return nil
	}
	names := map[string]bool{}
	fh, err := os.Open(path)
	if err != nil {
		return names
	}
	defer fh.Close()
	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		if n := strings.TrimSpace(scanner.Text()); n != "" {
			names[n] = true
		}
	}
	return names
}

func processCPU() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	user := time.Duration(ru.Utime.Sec)*time.Second + time.Duration(ru.Utime.Usec)*time.Microsecond
	sys := time.Duration(ru.Stime.Sec)*time.Second + time.Duration(ru.Stime.Usec)*time.Microsecond
	return user + sys
}

func main() {
	var (
		docsPath  = flag.String("docs", "", "TSV of doc id / name / chunk_num rows (required)")
		corpusDir = flag.String("corpus", "", "directory holding the source file named by each row (required)")
		outDir    = flag.String("out", "outputs/coverage", "output directory")
		esURL     = flag.String("es", "http://localhost:1200", "Elasticsearch base URL")
		esUser    = flag.String("es-user", "elastic", "Elasticsearch user")
		esPass    = flag.String("es-pass", "", "Elasticsearch password (default: ELASTIC_PASSWORD from --env)")
		envFile   = flag.String("env", "docker/.env", "file to read ELASTIC_PASSWORD from")
		index     = flag.String("index", "", "ES index to read (required)")
		kb        = flag.String("kb", "", "dataset id every checked document belongs to (required)")
		shards    = flag.Int("shards", 8, "parallel ES producer goroutines")
		workers   = flag.Int("workers", 0, "coverage workers (0 = number of CPUs)")
		pageSize  = flag.Int("page", 1000, "chunk documents per ES page")
		method    = flag.String("method", "alnum", "alnum (k-grams over letters/digits) or window (raw byte windows)")
		window    = flag.Int("window", 200, "byte window length for --method window")
		stride    = flag.Int("stride", 50, "byte window step for --method window")
		minGap    = flag.Int("min-gap", 200, "report uncovered runs at least this many bytes")
		limit     = flag.Int("limit", 0, "only the first N documents (calibration)")
		onlyFile  = flag.String("only-file", "", "only documents named in this file")
		every     = flag.Int("every", 200, "progress line every N documents")
	)
	flag.Parse()

	// Nothing about the document set is baked in. A default corpus or index would mean a run that
	// forgot a flag still produced numbers - about whatever was compiled in.
	var missing []string
	for _, required := range []struct {
		name  string
		value string
	}{
		{"docs", *docsPath},
		{"corpus", *corpusDir},
		{"index", *index},
		{"kb", *kb},
	} {
		if strings.TrimSpace(required.value) == "" {
			missing = append(missing, "-"+required.name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "required flag(s) missing: %s\n", strings.Join(missing, ", "))
		flag.Usage()
		os.Exit(2)
	}

	if *workers <= 0 {
		*workers = runtime.NumCPU()
	}
	if *esPass == "" {
		*esPass = envPassword(*envFile, "ELASTIC_PASSWORD")
	}
	if *esPass == "" {
		fmt.Fprintln(os.Stderr, "no ES password: pass --es-pass or set ELASTIC_PASSWORD in --env")
		os.Exit(2)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	opts := options{method: *method, window: *window, stride: *stride, minGap: *minGap}

	all, err := loadDocs(*docsPath, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	outPath := filepath.Join(*outDir, "coverage_go.jsonl")
	done := loadDone(outPath)
	only := loadNames(*onlyFile)
	todo := make([]doc, 0, len(all))
	for _, d := range all {
		if done[d.Name] {
			continue
		}
		if len(only) > 0 && !only[d.Name] {
			continue
		}
		todo = append(todo, d)
	}
	fmt.Printf("docs=%d already=%d todo=%d shards=%d workers=%d method=%s min-gap=%d bytes\n",
		len(all), len(done), len(todo), *shards, *workers, opts.method, opts.minGap)

	// Shard the work into contiguous blocks so each shard's ES pages are a narrow slice of the
	// index rather than a random sprinkle of documents.
	shardSets := make([][]doc, 0, *shards)
	per := (len(todo) + *shards - 1) / max(*shards, 1)
	for start := 0; start < len(todo); start += per {
		shardSets = append(shardSets, todo[start:min(start+per, len(todo))])
	}

	es := newESClient(*esURL, *esUser, *esPass, *shards)
	st := &stats{}
	jobs := make(chan job, *workers*2)
	results := make(chan result, *workers*2)
	var producerWG sync.WaitGroup
	for _, shard := range shardSets {
		if len(shard) == 0 {
			continue
		}
		producerWG.Add(1)
		go produceShard(es, *index, *kb, shard, *pageSize, jobs, st, &producerWG)
	}
	go func() { producerWG.Wait(); close(jobs) }()

	var workerWG sync.WaitGroup
	for i := 0; i < *workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for j := range jobs {
				results <- measure(*corpusDir, j, opts, st)
			}
		}()
	}
	go func() { workerWG.Wait(); close(results) }()

	sink, err := os.OpenFile(outPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	buffered := bufio.NewWriter(sink)
	started := time.Now()
	var measured int64
	lossy := make([]result, 0, 64)
	glued := make([]result, 0, 64)
	for res := range results {
		payload, _ := json.Marshal(res)
		buffered.Write(payload)
		buffered.WriteByte('\n')
		if atomic.AddInt64(&measured, 1)%10 == 0 {
			buffered.Flush()
		}
		// The verdict is strict: a word that exists only glued inside a longer token is not served
		// either, so gluing is a defect rather than a lesser category. The absent/fused split is
		// kept only to tell the two failure modes apart.
		if res.Error != "" || res.MissingStrictWords > 0 {
			lossy = append(lossy, res)
		}
		if res.FusedWords > 0 {
			glued = append(glued, res)
		}
		n := atomic.LoadInt64(&measured)
		if int(n)%*every == 0 || int(n) == len(todo) {
			elapsed := time.Since(started).Seconds()
			rate := float64(n) / elapsed
			fmt.Printf("  %d/%d  %.1f docs/s  ETA %.1f min\n", n, len(todo), rate, float64(len(todo)-int(n))/rate/60)
		}
	}
	buffered.Flush()
	sink.Close()

	sort.Slice(lossy, func(i, j int) bool { return lossy[i].MissingStrictWords > lossy[j].MissingStrictWords })
	sort.Slice(glued, func(i, j int) bool { return glued[i].FusedWords > glued[j].FusedWords })
	totalStrict, totalAbsent, totalFused := 0, 0, 0
	for _, r := range lossy {
		totalStrict += r.MissingStrictWords
		totalAbsent += r.AbsentWords
	}
	for _, r := range glued {
		totalFused += r.FusedWords
	}
	fmt.Printf("\n=== summary: %d documents in %.1f min ===\n", measured, time.Since(started).Minutes())
	fmt.Printf("  documents with unserved words / errors : %d (sum %d words)\n", len(lossy), totalStrict)
	// Consistency check, not a warning to ignore: every glued document is by definition a document
	// with unserved words (gluing is not tolerated), so an empty strict verdict while glued
	// documents exist means the strict count was not populated - the exact way this report managed
	// to look clean while flagging thousands of glued words in its own next line.
	if len(lossy) == 0 && len(glued) > 0 {
		fmt.Printf("  INCONSISTENT: %d documents have glued words but none counted as unserved; the strict verdict is not wired\n", len(glued))
		os.Exit(1)
	}
	fmt.Printf("    of which content never arrived       : %d words\n", totalAbsent)
	fmt.Printf("    of which glued into a longer token   : %d words in %d documents (NOT tolerated)\n",
		totalFused, len(glued))
	for i, r := range lossy {
		if i < 20 {
			fmt.Printf("    STRICT %-16s unserved=%-6d (absent=%-5d glued=%-5d) words=%.2f%% chunks=%d %v\n",
				r.Name, r.MissingStrictWords, r.AbsentWords, r.FusedWords, r.WordPct, r.ChunkCount, r.StrictSamples)
		}
	}
	lossyNames := make([]string, 0, len(lossy))
	for _, r := range lossy {
		lossyNames = append(lossyNames, r.Name)
	}
	gluedNames := make([]string, 0, len(glued))
	for _, r := range glued {
		gluedNames = append(gluedNames, r.Name)
	}
	flagPath := filepath.Join(*outDir, "below99.txt")
	_ = os.WriteFile(flagPath, []byte(strings.Join(lossyNames, "\n")+"\n"), 0o644)
	fusedPath := filepath.Join(*outDir, "fused.txt")
	_ = os.WriteFile(fusedPath, []byte(strings.Join(gluedNames, "\n")+"\n"), 0o644)
	st.report(time.Since(started), processCPU())
	fmt.Printf("wrote %s, %s and %s\n", outPath, flagPath, fusedPath)
}
