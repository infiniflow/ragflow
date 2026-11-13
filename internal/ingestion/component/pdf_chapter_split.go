// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"ragflow/internal/common"
	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/parser/splitter"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var pdfChapterDispatchSingle = func(ctx context.Context, db *gorm.DB, filename string, binary []byte, tenantID string, setup schema.ParserSetup) (parser.ParseResult, bool, error) {
	if res, handled, err := maybeDispatchTenantPDFParser(ctx, db, filename, binary, tenantID, setup); handled {
		return res, true, err
	}
	return dispatchPDFBackend(ctx, db, filename, binary, tenantID, setup)
}

var errPDFChapterUnhandled = errors.New("parser: no backend handled pdf chapter")

func getNumericSetting(val any) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		if iv, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return iv, true
		}
	}
	return 0, false
}

// resolveSplitOptions parses SplitOptions from setup and environment variables.
func resolveSplitOptions(setup schema.ParserSetup) splitter.SplitOptions {
	opts := splitter.DefaultSplitOptions()

	if v, ok := setup["chapter_split_threshold"]; ok {
		if iv, ok := getNumericSetting(v); ok && iv > 0 {
			opts.ThresholdPages = iv
		}
	} else if envV := os.Getenv(common.EnvChapterSplitThreshold); envV != "" {
		if iv, err := strconv.Atoi(envV); err == nil && iv > 0 {
			opts.ThresholdPages = iv
		}
	}

	if v, ok := setup["chapter_min_pages"]; ok {
		if iv, ok := getNumericSetting(v); ok && iv > 0 {
			opts.MinPages = iv
		}
	} else if envV := os.Getenv(common.EnvChapterMinPages); envV != "" {
		if iv, err := strconv.Atoi(envV); err == nil && iv > 0 {
			opts.MinPages = iv
		}
	}

	if v, ok := setup["chapter_max_pages"]; ok {
		if iv, ok := getNumericSetting(v); ok && iv > 0 {
			opts.MaxPages = iv
		}
	} else if envV := os.Getenv(common.EnvChapterMaxPages); envV != "" {
		if iv, err := strconv.Atoi(envV); err == nil && iv > 0 {
			opts.MaxPages = iv
		}
	}

	if v, ok := setup["chapter_concurrency"]; ok {
		if iv, ok := getNumericSetting(v); ok && iv > 0 {
			opts.MaxConcurrency = iv
		}
	} else if envV := os.Getenv(common.EnvChapterConcurrency); envV != "" {
		if iv, err := strconv.Atoi(envV); err == nil && iv > 0 {
			opts.MaxConcurrency = iv
		}
	}

	return opts.Normalize()
}

func shouldAdaptiveChapterSplit(setup schema.ParserSetup, binary []byte) bool {
	enabled := false
	if v, ok := setup["enable_adaptive_chapter_split"]; ok {
		if b, ok := v.(bool); ok {
			enabled = b
		} else if s, ok := v.(string); ok {
			enabled = strings.EqualFold(s, "true") || s == "1"
		}
	} else if envV := os.Getenv(common.EnvEnableAdaptiveChapterSplit); envV != "" {
		enabled = strings.EqualFold(envV, "true") || envV == "1"
	}
	if !enabled {
		return false
	}

	opts := resolveSplitOptions(setup)
	pageCount := splitter.GetPDFPageCount(binary)
	return pageCount >= opts.ThresholdPages
}

// dispatchAdaptiveChapterPDF partitions a large PDF into adaptive chapters,
// processes them concurrently within bounded concurrency, and losslessly
// concatenates the results.
func dispatchAdaptiveChapterPDF(
	ctx context.Context,
	db *gorm.DB,
	filename string,
	binary []byte,
	tenantID string,
	setup schema.ParserSetup,
	normalize func(parser.ParseResult) parser.ParseResult,
) (parser.ParseResult, bool, error) {
	opts := resolveSplitOptions(setup)
	totalPages := splitter.GetPDFPageCount(binary)
	if totalPages < opts.ThresholdPages {
		return parser.ParseResult{}, false, nil
	}

	var outlines []doctype.Outline
	if o := parser.ExtractPDFOutlines(binary); len(o) > 0 {
		outlines = o
	}

	plans := splitter.PlanAdaptiveChapters(totalPages, outlines, opts)
	if len(plans) <= 1 {
		return parser.ParseResult{}, false, nil
	}

	common.Info("dispatchAdaptiveChapterPDF: executing adaptive chapter split",
		zap.String("filename", filename),
		zap.Int("total_pages", totalPages),
		zap.Int("chapter_count", len(plans)),
		zap.Int("concurrency", opts.MaxConcurrency))

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dispatchSingle := func(cname string, cdata []byte) (parser.ParseResult, error) {
		res, handled, err := pdfChapterDispatchSingle(cctx, db, cname, cdata, tenantID, setup)
		if !handled && err == nil {
			return parser.ParseResult{}, errPDFChapterUnhandled
		}
		return res, err
	}

	chapterResults := make([]parser.ParseResult, len(plans))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.MaxConcurrency)
	baseName := strings.TrimSuffix(filename, filepath.Ext(filename))
	var sliceErr error
	var firstErr error
	var firstErrOnce sync.Once

ChapterLoop:
	for i, plan := range plans {
		if cctx.Err() != nil {
			break ChapterLoop
		}

		chapterBytes, err := splitter.SlicePDF(binary, plan.StartPage, plan.EndPage)
		if err != nil {
			sliceErr = err
			cancel()
			break ChapterLoop
		}

		select {
		case sem <- struct{}{}:
		case <-cctx.Done():
			break ChapterLoop
		}

		wg.Add(1)
		go func(idx int, p splitter.ChapterPlan, cBytes []byte) {
			defer func() {
				<-sem
				wg.Done()
			}()

			cName := fmt.Sprintf("%s_Sec%02d_P%04d-%04d.pdf", baseName, p.Index+1, p.StartPage+1, p.EndPage+1)
			res, parseErr := dispatchSingle(cName, cBytes)
			if parseErr != nil {
				res.Err = parseErr
				firstErrOnce.Do(func() { firstErr = parseErr })
				cancel()
			}
			chapterResults[idx] = res
		}(i, plan, chapterBytes)
	}

	wg.Wait()

	if errors.Is(sliceErr, splitter.ErrSplitterNotSupported) {
		common.Info("dispatchAdaptiveChapterPDF: splitter not supported in this environment, falling back",
			zap.String("filename", filename))
		return parser.ParseResult{}, false, nil
	}
	if sliceErr != nil {
		return parser.ParseResult{}, true, sliceErr
	}

	for _, r := range chapterResults {
		if errors.Is(r.Err, errPDFChapterUnhandled) {
			common.Info("dispatchAdaptiveChapterPDF: backend does not handle PDF chapters, falling back",
				zap.String("filename", filename))
			return parser.ParseResult{}, false, nil
		}
	}

	if ctx.Err() != nil {
		return parser.ParseResult{Err: ctx.Err()}, true, ctx.Err()
	}

	if firstErr != nil {
		return parser.ParseResult{}, true, firstErr
	}

	for _, r := range chapterResults {
		if r.Err != nil {
			return parser.ParseResult{}, true, r.Err
		}
	}

	if cctx.Err() != nil {
		return parser.ParseResult{Err: cctx.Err()}, true, cctx.Err()
	}

	aggregated := splitter.ConcatenateResults(chapterResults, plans)
	if aggregated.Err != nil {
		return aggregated, true, aggregated.Err
	}

	aggregated = normalize(aggregated)
	return aggregated, true, aggregated.Err
}
