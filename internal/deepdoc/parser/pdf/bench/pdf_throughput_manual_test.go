//go:build cgo && manual

// Package bench holds manual, never-CI performance benchmarks for the DeepDoc
// PDF parser. It lives in its own package so it can be compiled and run
// independently of the parser package's other (occasionally work-in-progress)
// test files.
package bench

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/deepdoc/native"
	"ragflow/internal/deepdoc/parser/pdf"
	infnative "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMultiPDFThroughput parses every PDF in DEEPDOC_BENCH_DIR as a single
// request and reports how long each document takes, plus its peak resident
// memory. Inference runs single-threaded per session (intraOpThreads is pinned
// to 1 in native/session.go), so the process-wide inference ceiling is exactly
// the number of concurrent Runs the parser admits — the page worker pool and
// the native inference gate are both sized to DeepDocConcurrency(). See
// pdf.parser_concurrency.go and native/inference_limit.go.
//
// Run:
//
//	DEEPDOC_BENCH_DIR=$DIR DEEPDOC_MODEL_DIR=$MODELS \
//	  ./build.sh --test-manual -run TestMultiPDFThroughput ./internal/deepdoc/parser/pdf/bench/...
//
// Results accumulate in DEEPDOC_BENCH_OUT (default /tmp/deepdoc_throughput.csv);
// each run reprints the combined table, so a re-run with more documents shows the
// full set. Each document is parsed exactly once.
//
// Peak memory: while a document parses, a background sampler polls the process
// resident set size (VmRSS from /proc/self/status) every 50ms and records the
// maximum seen. RSS — not runtime.MemStats.HeapAlloc — is the right metric here
// because ONNX allocates model weights and inference tensors in C memory that Go
// heap stats never track. The reported peak therefore includes the once-loaded
// shared model weights plus the working set of that parse.
func TestMultiPDFThroughput(t *testing.T) {
	dir := os.Getenv("DEEPDOC_BENCH_DIR")
	if dir == "" {
		t.Skip("set DEEPDOC_BENCH_DIR to a directory of PDFs")
	}
	modelDir := os.Getenv("DEEPDOC_MODEL_DIR")
	if modelDir == "" {
		t.Skip("set DEEPDOC_MODEL_DIR")
	}
	cores := 1
	outPath := os.Getenv("DEEPDOC_BENCH_OUT")
	if outPath == "" {
		outPath = "/tmp/deepdoc_throughput.csv"
	}

	// Sessions run single-threaded (intraOpThreads is pinned to 1), so the
	// process inference ceiling is simply the parser's concurrency budget: both
	// the page worker pool and the native inference gate are sized to it.
	budget := pdf.DeepDocConcurrency()
	pdf.SetPageWorkerPoolSize(budget)
	native.SetInferenceLimit(budget)
	t.Logf("page worker pool = %d, inference gate = %d", budget, budget)

	// Collect PDFs, sorted for a stable doc ordering across runs.
	var paths []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no PDF files found in DEEPDOC_BENCH_DIR")
	}

	// Load all PDFs once so disk I/O is not part of the measured parse time.
	type doc struct {
		name string
		data []byte
	}
	docs := make([]doc, len(paths))
	for i, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		docs[i] = doc{name: filepath.Base(p), data: data}
	}

	if err := native.InitORT(); err != nil {
		t.Fatal(err)
	}
	analyzer, err := infnative.NewAnalyzer(modelDir, infnative.DefaultDropScore)
	if err != nil {
		t.Fatal(err)
	}
	parser := pdf.NewParser(pdftype.DefaultParserConfig())

	// Parse each document exactly once, timing the whole request and sampling
	// peak resident memory. Each document's row is flushed to the shared CSV
	// immediately after it parses, so a long run interrupted mid-batch still
	// keeps the documents it finished.
	type rec struct {
		doc     string
		bytes   int
		seconds float64
		rss     uint64
	}
	rows := make([]rec, 0, len(docs))
	for _, d := range docs {
		peakRSS, stopSampler := startPeakRSS(0)
		start := time.Now()
		res, err := parser.Parse(t.Context(), d.data, analyzer)
		elapsed := time.Since(start)
		peak := peakRSS()
		stopSampler()
		if err != nil {
			t.Fatalf("parse %s: %v", d.name, err)
		}
		res.Close()
		rows = append(rows, rec{doc: d.name, bytes: len(d.data), seconds: elapsed.Seconds(), rss: peak})
		t.Logf("[cores=%d] %s (%.2f MB) parsed in %.3fs, peak RSS %.1f MB",
			cores, d.name, float64(len(d.data))/1e6, elapsed.Seconds(), float64(peak)/1e6)

		// Flush incrementally: drop this run's prior rows, add everything
		// parsed so far, and rewrite the shared CSV.
		merged := loadThroughputCSV(t, outPath)
		merged = filterThroughputRows(merged, cores)
		for _, r := range rows {
			merged = append(merged, throughputRow{cores: cores, doc: r.doc, bytes: r.bytes,
				seconds: r.seconds, rss: r.rss})
		}
		writeThroughputCSV(t, outPath, merged)
	}

	// Print the combined doc × cores table across all runs so far.
	merged := loadThroughputCSV(t, outPath)
	printThroughputTable(t, merged)
}

// throughputRow is one (cores, doc) parse-time measurement.
type throughputRow struct {
	cores   int
	doc     string
	bytes   int
	seconds float64
	rss     uint64
}

func loadThroughputCSV(t *testing.T, path string) []throughputRow {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		t.Logf("warning: could not parse %s: %v", path, err)
		return nil
	}
	out := make([]throughputRow, 0, len(records))
	for _, rec := range records {
		if len(rec) < 4 {
			continue
		}
		cores, err1 := strconv.Atoi(rec[0])
		nbytes, err2 := strconv.Atoi(rec[2])
		secs, err3 := strconv.ParseFloat(rec[3], 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		r := throughputRow{cores: cores, doc: rec[1], bytes: nbytes, seconds: secs}
		if len(rec) >= 5 {
			// Older CSVs without the rss column are tolerated.
			if rss, err4 := strconv.ParseUint(rec[4], 10, 64); err4 == nil {
				r.rss = rss
			}
		}
		out = append(out, r)
	}
	return out
}

func filterThroughputRows(rows []throughputRow, cores int) []throughputRow {
	out := make([]throughputRow, 0, len(rows))
	for _, r := range rows {
		if r.cores != cores {
			out = append(out, r)
		}
	}
	return out
}

func writeThroughputCSV(t *testing.T, path string, rows []throughputRow) {
	f, err := os.Create(path)
	if err != nil {
		t.Logf("warning: could not write %s: %v", path, err)
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	header := []string{"cores", "doc", "bytes", "seconds", "rss_bytes"}
	if err := w.Write(header); err != nil {
		t.Logf("warning: csv header: %v", err)
	}
	for _, r := range rows {
		if err := w.Write([]string{
			strconv.Itoa(r.cores),
			r.doc,
			strconv.Itoa(r.bytes),
			strconv.FormatFloat(r.seconds, 'f', 6, 64),
			strconv.FormatUint(r.rss, 10),
		}); err != nil {
			t.Logf("warning: csv row: %v", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Logf("warning: csv flush: %v", err)
	}
}

func printThroughputTable(t *testing.T, rows []throughputRow) {
	docOrder := make([]string, 0)
	docSeen := make(map[string]bool)
	coreSet := make(map[int]bool)
	timeLookup := make(map[string]map[int]float64)
	rssLookup := make(map[string]map[int]uint64)
	for _, r := range rows {
		if !docSeen[r.doc] {
			docSeen[r.doc] = true
			docOrder = append(docOrder, r.doc)
		}
		coreSet[r.cores] = true
		if timeLookup[r.doc] == nil {
			timeLookup[r.doc] = make(map[int]float64)
		}
		if rssLookup[r.doc] == nil {
			rssLookup[r.doc] = make(map[int]uint64)
		}
		timeLookup[r.doc][r.cores] = r.seconds
		rssLookup[r.doc][r.cores] = r.rss
	}
	cores := make([]int, 0, len(coreSet))
	for c := range coreSet {
		cores = append(cores, c)
	}
	sort.Ints(cores)

	t.Log("── combined parse time (seconds), doc × cores ──")
	header := "doc"
	for _, c := range cores {
		header += fmt.Sprintf("\t%d-core", c)
	}
	t.Log(header)
	for _, d := range docOrder {
		line := d
		for _, c := range cores {
			if s, ok := timeLookup[d][c]; ok {
				line += fmt.Sprintf("\t%.3f", s)
			} else {
				line += "\t-"
			}
		}
		t.Log(line)
	}

	t.Log("── peak RSS (MB), doc × cores ──")
	t.Log(header)
	for _, d := range docOrder {
		line := d
		for _, c := range cores {
			if s, ok := rssLookup[d][c]; ok {
				line += fmt.Sprintf("\t%.1f", float64(s)/1e6)
			} else {
				line += "\t-"
			}
		}
		t.Log(line)
	}
}

// startPeakRSS spawns a goroutine that polls the process resident set size
// (VmRSS from /proc/self/status) every sampleInterval and records the maximum
// seen. RSS — not runtime.MemStats.HeapAlloc — is the metric that captures the
// real peak of a parse, because ONNX allocates model weights and inference
// tensors in C memory that Go heap stats never track. The returned peak func
// reads the current peak; stop ends sampling. Pass a non-positive interval for
// the default 50ms.
func startPeakRSS(sampleInterval time.Duration) (peak func() uint64, stop func()) {
	var peakBytes atomic.Uint64
	if sampleInterval <= 0 {
		sampleInterval = 50 * time.Millisecond
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(sampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if rss := readRSSBytes(); rss > peakBytes.Load() {
					peakBytes.Store(rss)
				}
			}
		}
	}()
	return peakBytes.Load, func() { close(done) }
}

// readRSSBytes parses VmRSS (in bytes) from /proc/self/status. It returns 0 if
// the file is unavailable (non-Linux or unreadable) so callers degrade
// gracefully and report "peak RSS 0 MB" rather than failing the benchmark.
func readRSSBytes() uint64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}
