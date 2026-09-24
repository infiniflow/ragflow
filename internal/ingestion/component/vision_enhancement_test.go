//
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
//

package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

type visionEnhanceFakeDriver struct {
	modelModule.ModelDriver
}

func TestMediaOCRStatus_UsesPerItemMarker(t *testing.T) {
	tests := []struct {
		name        string
		fileType    utility.FileType
		parseMethod string
		item        map[string]any
		want        ocrStatus
	}{
		{
			name:        "pending marker overrides deepdoc table default",
			fileType:    utility.FileTypePDF,
			parseMethod: "deepdoc",
			item:        map[string]any{"doc_type_kwd": "table", "ocr_status_kwd": "pending"},
			want:        ocrPending,
		},
		{
			name:        "attempted marker overrides external image default",
			fileType:    utility.FileTypePDF,
			parseMethod: "mineru",
			item:        map[string]any{"doc_type_kwd": "image", "ocr_status_kwd": "attempted"},
			want:        ocrAttempted,
		},
		{
			name:        "unknown marker overrides deepdoc image default",
			fileType:    utility.FileTypePDF,
			parseMethod: "deepdoc",
			item:        map[string]any{"doc_type_kwd": "image", "ocr_status_kwd": "unknown"},
			want:        ocrUnknown,
		},
		{
			name:        "unrecognized marker falls back to existing policy",
			fileType:    utility.FileTypePDF,
			parseMethod: "deepdoc",
			item:        map[string]any{"doc_type_kwd": "table", "ocr_status_kwd": "later"},
			want:        ocrAttempted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mediaOCRStatus(tt.fileType, tt.parseMethod, tt.item); got != tt.want {
				t.Errorf("mediaOCRStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveHTMLImageSourceLoadsRelativeAsset(t *testing.T) {
	imagePayload := visionTestPNGBase64(t)
	imageBytes, err := base64.StdEncoding.DecodeString(imagePayload)
	if err != nil {
		t.Fatalf("decode test PNG: %v", err)
	}
	storage := withMemoryStorage(t)
	if err := storage.Put(t.Context(), "html-assets", "docs/images/chart.png", imageBytes); err != nil {
		t.Fatalf("seed relative image: %v", err)
	}

	item := map[string]any{
		"text":            "chart alt",
		"doc_type_kwd":    "image",
		"image_src":       "images/chart.png",
		"parent_table_id": "html-table-1",
	}
	if !resolveHTMLImageSource(t.Context(), "html-assets", "docs/report.html", item) {
		t.Fatal("resolveHTMLImageSource() = false, want true")
	}
	if _, ok := item["image_src"]; ok {
		t.Fatalf("unresolved image source remains after storage lookup: %+v", item)
	}
	if image, _ := item["image"].(string); !strings.HasPrefix(image, "data:image/png;base64,") {
		t.Fatalf("image payload = %q, want PNG data URI", image)
	}
	if item["text"] != "chart alt" || item["parent_table_id"] != "html-table-1" {
		t.Fatalf("resource metadata changed during resolution: %+v", item)
	}
}

func TestResolveHTMLImageSourceLoadsParentRelativeAssetWithinBucket(t *testing.T) {
	imagePayload := visionTestPNGBase64(t)
	imageBytes, err := base64.StdEncoding.DecodeString(imagePayload)
	if err != nil {
		t.Fatalf("decode test PNG: %v", err)
	}
	storage := withMemoryStorage(t)
	if err := storage.Put(t.Context(), "html-assets", "assets/chart.png", imageBytes); err != nil {
		t.Fatalf("seed parent-relative image: %v", err)
	}

	item := map[string]any{"doc_type_kwd": "image", "image_src": "../assets/chart.png"}
	if !resolveHTMLImageSource(t.Context(), "html-assets", "docs/report.html", item) {
		t.Fatal("resolveHTMLImageSource() = false, want parent-relative asset to resolve within bucket")
	}
	if _, ok := item["image_src"]; ok {
		t.Fatalf("image_src remains after resolution: %+v", item)
	}
}

func TestVisionEnhancementLoadsRelativeHTMLImageForOCR(t *testing.T) {
	analyzer := &requestContextAnalyzer{}
	useRequestContextAnalyzer(t, analyzer)
	imagePayload := visionTestPNGBase64(t)
	imageBytes, err := base64.StdEncoding.DecodeString(imagePayload)
	if err != nil {
		t.Fatalf("decode test PNG: %v", err)
	}
	storage := withMemoryStorage(t)
	if err := storage.Put(t.Context(), "html-assets", "docs/images/chart.png", imageBytes); err != nil {
		t.Fatalf("seed relative image: %v", err)
	}
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{{
			"text":         "chart alt",
			"doc_type_kwd": "image",
			"image_src":    "images/chart.png",
		}},
	}

	result, handled, err := maybeDispatchVisionEnhancement(
		t.Context(), dao.DB, utility.FileTypeHTML, dispatched,
		map[string]any{"bucket": "html-assets", "path": "docs/report.html"},
		map[string]schema.ParserSetup{"html": {}},
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want relative image OCR")
	}
	if analyzer.detectCalls != 1 {
		t.Errorf("OCR detect calls = %d, want 1", analyzer.detectCalls)
	}
	if _, ok := result.JSON[0]["image_src"]; ok {
		t.Errorf("relative source not materialized: %+v", result.JSON[0])
	}
	if got := result.JSON[0]["text"]; got != "chart alt\n"+strings.TrimSpace(strings.Repeat("recognized ", 4)) {
		t.Errorf("image text = %q, want alt plus OCR result", got)
	}
}

func TestResolveRelativeHTMLImagePathStaysWithinHTMLDirectory(t *testing.T) {
	if got, ok := resolveRelativeHTMLImagePath("docs/report.html", "images/chart.png"); !ok || got != "docs/images/chart.png" {
		t.Errorf("resolveRelativeHTMLImagePath() = %q, %v; want docs/images/chart.png, true", got, ok)
	}
	if got, ok := resolveRelativeHTMLImagePath("docs/report.html", "images/my chart.png"); !ok || got != "docs/images/my chart.png" {
		t.Errorf("space path = %q, %v; want docs/images/my chart.png, true", got, ok)
	}
	if got, ok := resolveRelativeHTMLImagePath("docs/report.html", "../assets/chart.png"); !ok || got != "assets/chart.png" {
		t.Errorf("parent-relative path = %q, %v; want assets/chart.png, true", got, ok)
	}
	if got, ok := resolveRelativeHTMLImagePath("docs/report.html", "../../private.png"); ok {
		t.Errorf("path escape resolved to %q", got)
	}
}

type concurrentVisionOCRAnalyzer struct {
	active atomic.Int32
	peak   atomic.Int32
}

type budgetVisionOCRAnalyzer struct {
	detectCalls atomic.Int32
}

func (*budgetVisionOCRAnalyzer) DLA(context.Context, image.Image) ([]deepdoctype.DLARegion, error) {
	return nil, nil
}

func (*budgetVisionOCRAnalyzer) TSR(context.Context, image.Image) ([]deepdoctype.TSRCell, error) {
	return nil, nil
}

func (a *budgetVisionOCRAnalyzer) OCRDetect(ctx context.Context, _ image.Image) ([]deepdoctype.OCRBox, error) {
	a.detectCalls.Add(1)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*budgetVisionOCRAnalyzer) OCRRecognize(context.Context, image.Image) ([]deepdoctype.OCRText, error) {
	return nil, nil
}

func (*budgetVisionOCRAnalyzer) Health() bool { return true }

func (*concurrentVisionOCRAnalyzer) DLA(context.Context, image.Image) ([]deepdoctype.DLARegion, error) {
	return nil, nil
}

func (*concurrentVisionOCRAnalyzer) TSR(context.Context, image.Image) ([]deepdoctype.TSRCell, error) {
	return nil, nil
}

func (a *concurrentVisionOCRAnalyzer) OCRDetect(context.Context, image.Image) ([]deepdoctype.OCRBox, error) {
	active := a.active.Add(1)
	for peak := a.peak.Load(); active > peak; peak = a.peak.Load() {
		if a.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	a.active.Add(-1)
	return []deepdoctype.OCRBox{{X0: 1, Y0: 1, X1: 19, Y1: 1, X2: 19, Y2: 19, X3: 1, Y3: 19}}, nil
}

func (*concurrentVisionOCRAnalyzer) OCRRecognize(context.Context, image.Image) ([]deepdoctype.OCRText, error) {
	return []deepdoctype.OCRText{{Text: "local text"}}, nil
}

func (*concurrentVisionOCRAnalyzer) Health() bool { return true }

type visionEnhanceCaptureInvoker struct {
	mu       sync.Mutex
	images   []string
	captured []modelModule.Message
}

func (c *visionEnhanceCaptureInvoker) invoke(
	ctx context.Context,
	driver modelModule.ModelDriver,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
) (*modelModule.ChatResponse, error) {
	c.mu.Lock()
	c.captured = append(c.captured, messages...)
	if parts, ok := messages[0].Content.([]interface{}); ok && len(parts) >= 2 {
		if img, ok := parts[1].(map[string]any); ok {
			if url, ok := img["image_url"].(map[string]any); ok {
				if u, ok := url["url"].(string); ok {
					c.images = append(c.images, u)
				}
			}
		}
	}
	c.mu.Unlock()
	ans := "```markdown\na diagram of a pipeline\n```"
	return &modelModule.ChatResponse{Answer: &ans}, nil
}

// swapVisionGlobals replaces injectable vars and restores them on t.Cleanup.
func swapVisionGlobals(
	t *testing.T,
	resolver func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error),
	invoker func(context.Context, modelModule.ModelDriver, string, []modelModule.Message, *modelModule.APIConfig) (*modelModule.ChatResponse, error),
	prompt func(string) (string, error),
) {
	t.Helper()
	origResolver := resolveTenantModelByType
	origInvoker := visionChatInvoker
	origPrompt := figureVisionPromptBuilder
	t.Cleanup(func() {
		resolveTenantModelByType = origResolver
		visionChatInvoker = origInvoker
		figureVisionPromptBuilder = origPrompt
	})
	if resolver != nil {
		resolveTenantModelByType = resolver
	}
	if invoker != nil {
		visionChatInvoker = invoker
	}
	if prompt != nil {
		figureVisionPromptBuilder = prompt
	}
}

func fakeResolver(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	return &visionEnhanceFakeDriver{}, "vision-model", &modelModule.APIConfig{}, 0, nil
}

func fakePrompt(language string) (string, error) {
	return "describe the figure in " + language, nil
}

func visionTestPNGBase64(t *testing.T) string {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 20, 20))); err != nil {
		t.Fatalf("encode image: %v", err)
	}
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func TestVisionEnhancement_AppendsLocalOCRBeforeVLM(t *testing.T) {
	analyzer := &requestContextAnalyzer{}
	useRequestContextAnalyzer(t, analyzer)
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{{
			"text":         "Existing caption",
			"image":        visionTestPNGBase64(t),
			"doc_type_kwd": "image",
		}},
	}
	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(), dao.DB, utility.FileTypeXLSX, dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	want := "Existing caption\n" + strings.TrimSpace(strings.Repeat("recognized ", 4)) + "\na diagram of a pipeline"
	if got := res.JSON[0]["text"]; got != want {
		t.Errorf("enhanced text = %q, want %q", got, want)
	}
	if analyzer.detectCalls != 1 || analyzer.recognizeCalls != 1 {
		t.Errorf("OCR calls = detect %d, recognize %d; want 1 each", analyzer.detectCalls, analyzer.recognizeCalls)
	}
	if len(invoker.images) != 1 {
		t.Errorf("VLM calls = %d, want 1", len(invoker.images))
	}
}

func TestVisionEnhancement_SkipsOCRForPDFTableSourcesThatAlreadyTriedOrUnknown(t *testing.T) {
	for _, parseMethod := range []string{"deepdoc", "mineru"} {
		t.Run(parseMethod, func(t *testing.T) {
			analyzer := &requestContextAnalyzer{}
			useRequestContextAnalyzer(t, analyzer)
			invoker := &visionEnhanceCaptureInvoker{}
			swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

			dispatched := parser.ParseResult{
				OutputFormat: "json",
				JSON: []map[string]any{{
					"text":         "Existing table",
					"image":        visionTestPNGBase64(t),
					"doc_type_kwd": "table",
				}},
			}
			res, handled, err := maybeDispatchVisionEnhancement(
				t.Context(), dao.DB, utility.FileTypePDF, dispatched,
				map[string]any{"tenant_id": "t1"},
				map[string]schema.ParserSetup{"pdf": {"parse_method": parseMethod}},
			)
			if err != nil {
				t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
			}
			if !handled {
				t.Fatal("handled = false, want VLM enhancement")
			}
			if analyzer.detectCalls != 0 {
				t.Errorf("OCR detect calls = %d, want 0 for parse_method %q", analyzer.detectCalls, parseMethod)
			}
			if got, want := res.JSON[0]["text"], "Existing table\na diagram of a pipeline"; got != want {
				t.Errorf("enhanced text = %q, want %q", got, want)
			}
		})
	}
}

func TestVisionEnhancement_TableCellImageUsesItsOwnOCRStatus(t *testing.T) {
	analyzer := &requestContextAnalyzer{}
	useRequestContextAnalyzer(t, analyzer)
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
	imagePayload := visionTestPNGBase64(t)
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "<table><tr><td>figure</td></tr></table>", "image": imagePayload, "doc_type_kwd": "table"},
			{
				"text":            "cell alt",
				"image":           imagePayload,
				"doc_type_kwd":    "image",
				"parent_table_id": "html-table-1",
				"row_index":       1,
				"column_index":    1,
				"media_order":     1,
			},
		},
	}

	result, handled, err := maybeDispatchVisionEnhancement(
		t.Context(), dao.DB, utility.FileTypePDF, dispatched,
		map[string]any{"tenant_id": "t1"}, map[string]schema.ParserSetup{"pdf": {"parse_method": "deepdoc"}},
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want table and cell image VLM processing")
	}
	if analyzer.detectCalls != 1 {
		t.Errorf("OCR detect calls = %d, want only the independent cell image", analyzer.detectCalls)
	}
	if got, want := result.JSON[0]["text"], "<table><tr><td>figure</td></tr></table>\na diagram of a pipeline"; got != want {
		t.Errorf("table text = %q, want %q", got, want)
	}
	cellWant := "cell alt\n" + strings.TrimSpace(strings.Repeat("recognized ", 4)) + "\na diagram of a pipeline"
	if got := result.JSON[1]["text"]; got != cellWant {
		t.Errorf("cell image text = %q, want %q", got, cellWant)
	}
	if len(invoker.images) != 2 {
		t.Errorf("VLM calls = %d, want table plus cell image", len(invoker.images))
	}
}

func TestVisionEnhancement_UnknownPDFImageRunsVLMWithoutLocalOCR(t *testing.T) {
	analyzer := &requestContextAnalyzer{}
	useRequestContextAnalyzer(t, analyzer)
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON:         []map[string]any{{"text": "existing", "image": visionTestPNGBase64(t), "doc_type_kwd": "image"}},
	}

	result, handled, err := maybeDispatchVisionEnhancement(
		t.Context(), dao.DB, utility.FileTypePDF, dispatched,
		map[string]any{"tenant_id": "t1"}, map[string]schema.ParserSetup{"pdf": {"parse_method": "mineru"}},
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want VLM enhancement")
	}
	if analyzer.detectCalls != 0 {
		t.Errorf("OCR detect calls = %d, want 0 for unknown parser text source", analyzer.detectCalls)
	}
	if got, want := result.JSON[0]["text"], "existing\na diagram of a pipeline"; got != want {
		t.Errorf("image text = %q, want %q", got, want)
	}
	if len(invoker.images) != 1 {
		t.Errorf("VLM calls = %d, want 1", len(invoker.images))
	}
}

func TestVisionEnhancement_RunsOCRWithoutTenantForVLM(t *testing.T) {
	analyzer := &requestContextAnalyzer{}
	useRequestContextAnalyzer(t, analyzer)
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{{
			"text":         "Existing caption",
			"image":        visionTestPNGBase64(t),
			"doc_type_kwd": "image",
		}},
	}
	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(), dao.DB, utility.FileTypeXLSX, dispatched, nil,
		map[string]schema.ParserSetup{"xlsx": {}},
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want local OCR to modify the item")
	}
	if analyzer.detectCalls != 1 || analyzer.recognizeCalls != 1 {
		t.Errorf("OCR calls = detect %d, recognize %d; want 1 each", analyzer.detectCalls, analyzer.recognizeCalls)
	}
	if len(invoker.images) != 0 {
		t.Errorf("VLM calls = %d, want 0 without tenant_id", len(invoker.images))
	}
	want := "Existing caption\n" + strings.TrimSpace(strings.Repeat("recognized ", 4))
	if got := res.JSON[0]["text"]; got != want {
		t.Errorf("enhanced text = %q, want %q", got, want)
	}
}

func TestVisionEnhancement_BoundsConcurrentOCRMediaAcrossInvokes(t *testing.T) {
	analyzer := &concurrentVisionOCRAnalyzer{}
	originalFactory := deepdoctype.NativeDocAnalyzerFactory
	deepdoctype.NativeDocAnalyzerFactory = func() (deepdoctype.DocAnalyzer, bool) { return analyzer, true }
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = originalFactory })

	imagePayload := visionTestPNGBase64(t)
	limit := deepdocpdf.DeepDocConcurrency()
	invokes := limit + 2
	errs := make(chan error, invokes)
	var wg sync.WaitGroup
	for range invokes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatched := parser.ParseResult{
				OutputFormat: "json",
				JSON:         []map[string]any{{"text": "", "image": imagePayload, "doc_type_kwd": "image"}},
			}
			_, handled, err := maybeDispatchVisionEnhancement(
				t.Context(), dao.DB, utility.FileTypeXLSX, dispatched, nil,
				map[string]schema.ParserSetup{"xlsx": {}},
			)
			if err != nil {
				errs <- err
			} else if !handled {
				errs <- fmt.Errorf("enhancement was not handled")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent enhancement: %v", err)
	}
	if got := analyzer.peak.Load(); got > int32(limit) {
		t.Errorf("peak OCR media tasks = %d, want at most configured limit %d", got, limit)
	}
}

func TestVisionEnhancement_OCRBudgetBoundsAdmissionWait(t *testing.T) {
	admission := sharedOCRMediaAdmission()
	releases := make([]func(), 0, cap(admission.slots))
	for range cap(admission.slots) {
		release, err := admission.acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	done := make(chan error, 1)
	finished := false
	defer func() {
		for _, release := range releases {
			release()
		}
		if !finished {
			<-done
		}
	}()

	originalInvokeBudget := visionOCRInvokeBudget
	visionOCRInvokeBudget = 100 * time.Millisecond
	t.Cleanup(func() { visionOCRInvokeBudget = originalInvokeBudget })
	result := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{{
			"doc_type_kwd": "image",
			"image":        visionTestPNGBase64(t),
		}},
	}
	go func() {
		_, _, err := maybeDispatchVisionEnhancement(
			t.Context(), nil, utility.FileTypeXLSX, result, nil,
			map[string]schema.ParserSetup{"xlsx": {}},
		)
		done <- err
	}()

	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("media admission wait exceeded the 100ms OCR invoke budget")
	}
}

func TestVisionEnhancement_OCRInvokeBudgetFallsBackToVLM(t *testing.T) {
	analyzer := &budgetVisionOCRAnalyzer{}
	originalFactory := deepdoctype.NativeDocAnalyzerFactory
	deepdoctype.NativeDocAnalyzerFactory = func() (deepdoctype.DocAnalyzer, bool) { return analyzer, true }
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = originalFactory })

	originalInvokeBudget, originalItemBudget := visionOCRInvokeBudget, visionOCRItemBudget
	visionOCRInvokeBudget = 20 * time.Millisecond
	visionOCRItemBudget = time.Second
	t.Cleanup(func() {
		visionOCRInvokeBudget = originalInvokeBudget
		visionOCRItemBudget = originalItemBudget
	})

	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
	imagePayload := visionTestPNGBase64(t)
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "first", "image": imagePayload, "doc_type_kwd": "image"},
			{"text": "second", "image": imagePayload, "doc_type_kwd": "image"},
		},
	}

	result, handled, err := maybeDispatchVisionEnhancement(
		t.Context(), dao.DB, utility.FileTypeXLSX, dispatched,
		map[string]any{"tenant_id": "t1"}, map[string]schema.ParserSetup{"xlsx": {}},
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want VLM fallback after the OCR budget expires")
	}
	if got := analyzer.detectCalls.Load(); got != 1 {
		t.Errorf("OCR detect calls = %d, want one before the invoke budget expires", got)
	}
	if len(invoker.images) != 2 {
		t.Errorf("VLM calls = %d, want both images to use the parent context", len(invoker.images))
	}
	for i, want := range []string{"first\na diagram of a pipeline", "second\na diagram of a pipeline"} {
		if got := result.JSON[i]["text"]; got != want {
			t.Errorf("item %d text = %q, want %q", i, got, want)
		}
	}
}

func TestVisionEnhancement_EnhancesJSONImagesAndTables(t *testing.T) {
	testCases := []struct {
		name     string
		fileType utility.FileType
	}{
		{"DOCX", utility.FileTypeDOCX},
		{"PDF", utility.FileTypePDF},
		{"Markdown", utility.FileTypeMarkdown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedLanguage string
			// Per-subtest invoker and prompt — avoids implicit coupling between subtests.
			invoker := &visionEnhanceCaptureInvoker{}
			swapVisionGlobals(t, fakeResolver, invoker.invoke, func(language string) (string, error) {
				capturedLanguage = language
				return "describe the figure in " + language, nil
			})

			dispatched := parser.ParseResult{
				OutputFormat: "json",
				JSON: []map[string]any{
					{"text": "Intro paragraph", "image": nil, "doc_type_kwd": "text"},
					{"text": "", "image": "aGVsbG8taW1hZ2U=", "doc_type_kwd": "image"},
					{"text": "existing table", "image": "dGFibGUtaW1hZ2U=", "doc_type_kwd": "table"},
					{"text": "<table></table>", "image": nil, "doc_type_kwd": "table"},
				},
			}

			res, handled, err := maybeDispatchVisionEnhancement(
				t.Context(),
				dao.DB,
				tc.fileType,
				dispatched,
				map[string]any{"tenant_id": "t1", "lang": "Japanese"}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatal("handled = false, want true")
			}
			if len(res.JSON) != 4 {
				t.Fatalf("JSON len = %d, want 4", len(res.JSON))
			}
			if got := res.JSON[0]["text"].(string); got != "Intro paragraph" {
				t.Errorf("text item text = %q, want unchanged", got)
			}
			// image item: VLM description cleaned of ```markdown and appended
			if got, _ := res.JSON[1]["text"].(string); got != "a diagram of a pipeline" {
				t.Errorf("image item text = %q, want 'a diagram of a pipeline'", got)
			}
			// table item with image: VLM description appended with single \n
			if got, _ := res.JSON[2]["text"].(string); got != "existing table\na diagram of a pipeline" {
				t.Errorf("table item text = %q, want 'existing table\\na diagram of a pipeline'", got)
			}
			if got, _ := res.JSON[3]["text"].(string); got != "<table></table>" {
				t.Errorf("table item without image text = %q, want unchanged", got)
			}
			if capturedLanguage != "Japanese" {
				t.Errorf("figure prompt language = %q, want Japanese", capturedLanguage)
			}
		})
	}
}

// TestVisionEnhancement_LanguagePriority pins the prompt-language chain:
// run-level dataset language (inputs) > family setup lang > English. The
// setup fallback is what makes the DSL's pdf.lang effective, mirroring
// Python's conf.get("lang") in parser.py:778.
func TestVisionEnhancement_LanguagePriority(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		setups map[string]schema.ParserSetup
		want   string
	}{
		{
			name:   "dataset language wins over setup lang",
			inputs: map[string]any{"tenant_id": "t1", "lang": "Japanese"},
			setups: map[string]schema.ParserSetup{"pdf": {"lang": "Chinese"}},
			want:   "Japanese",
		},
		{
			name:   "setup lang used when inputs carry no lang",
			inputs: map[string]any{"tenant_id": "t1"},
			setups: map[string]schema.ParserSetup{"pdf": {"lang": "Chinese"}},
			want:   "Chinese",
		},
		{
			name:   "english when neither is set",
			inputs: map[string]any{"tenant_id": "t1"},
			setups: nil,
			want:   "English",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedLanguage string
			swapVisionGlobals(t, fakeResolver, (&visionEnhanceCaptureInvoker{}).invoke,
				func(language string) (string, error) {
					capturedLanguage = language
					return "describe the figure in " + language, nil
				})

			dispatched := parser.ParseResult{
				OutputFormat: "json",
				JSON: []map[string]any{
					{"text": "", "image": "aGVsbG8taW1hZ2U=", "doc_type_kwd": "image"},
				},
			}

			_, handled, err := maybeDispatchVisionEnhancement(
				t.Context(), dao.DB, utility.FileTypePDF, dispatched, tc.inputs, tc.setups)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatal("handled = false, want true")
			}
			if capturedLanguage != tc.want {
				t.Errorf("figure prompt language = %q, want %q", capturedLanguage, tc.want)
			}
		})
	}
}

func TestVisionEnhancement_MarkdownOutputUntouched(t *testing.T) {
	called := false
	swapVisionGlobals(t,
		func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
			called = true
			return &visionEnhanceFakeDriver{}, "m", &modelModule.APIConfig{}, 0, nil
		},
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			called = true
			ans := "x"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		nil,
	)

	dispatched := parser.ParseResult{
		OutputFormat: "markdown",
		Markdown:     "![Image](data:image/png;base64,abc)",
		File:         map[string]any{"figures": []map[string]any{{"image": "abc", "marker": "x"}}},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypeDOCX,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false (markdown path must not be enhanced)")
	}
	if called {
		t.Error("vision model was resolved/invoked on the markdown path")
	}
	if res.Markdown != "![Image](data:image/png;base64,abc)" {
		t.Errorf("markdown mutated: %q", res.Markdown)
	}
}

func TestVisionEnhancement_UsesVisualPayloadRegardlessOfFileType(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "caption", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypeOTHER,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true for a visual payload in FileTypeOTHER")
	}
	if got, want := res.JSON[0]["text"], "caption\na diagram of a pipeline"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if len(invoker.images) != 1 {
		t.Fatalf("VLM calls = %d, want 1", len(invoker.images))
	}
}

func TestVisionEnhancement_EmptyOrNoTenantSkipped(t *testing.T) {
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	// No tenant_id
	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypeDOCX,
		dispatched,
		map[string]any{}, nil)
	if err != nil || handled {
		t.Errorf("handled=%v, err=%v, want false, nil for missing tenant_id", handled, err)
	}
	if res.JSON[0]["text"] != "" {
		t.Errorf("text = %q, want untouched", res.JSON[0]["text"])
	}
}

// TestVisionEnhancement_DispatchedErrSkipped ensures a failed parse result is
// returned unchanged — enhancement must not touch items when dispatched.Err != nil.
func TestVisionEnhancement_DispatchedErrSkipped(t *testing.T) {
	parseErr := errors.New("parse failed")
	dispatched := parser.ParseResult{
		Err:          parseErr,
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when dispatched.Err is set")
	}
	if res.Err != parseErr {
		t.Errorf("res.Err = %v, want original parse error preserved", res.Err)
	}
}

// TestVisionEnhancement_ContextCancellation verifies that a cancelled context
// propagates through visionChatInvoker and the enhancement is skipped without panic.
func TestVisionEnhancement_ContextCancellation(t *testing.T) {
	swapVisionGlobals(t, fakeResolver,
		func(ctx context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			return nil, ctx.Err()
		},
		fakePrompt,
	)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		ctx,
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if handled {
		t.Error("handled = true with cancelled context, want false")
	}
	// text field should be untouched since invoker returned no answer.
	if got, _ := res.JSON[0]["text"].(string); got != "" {
		t.Errorf("text = %q, want empty (no successful VLM response)", got)
	}
}

// TestVisionEnhancement_NonStringImageFieldFiltered verifies that items with a
// non-string image field (e.g., int) are silently skipped during target collection.
func TestVisionEnhancement_NonStringImageFieldFiltered(t *testing.T) {
	invokerCalled := false
	swapVisionGlobals(t, fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			invokerCalled = true
			ans := "description"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		fakePrompt,
	)

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			// non-string image field — filtered by target collector
			{"text": "para", "image": 12345, "doc_type_kwd": "image"},
			// nil image — also filtered
			{"text": "para2", "image": nil, "doc_type_kwd": "image"},
		},
	}

	_, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false — no valid image targets")
	}
	if invokerCalled {
		t.Error("invoker was called despite no valid image targets")
	}
}

// TestVisionEnhancement_MoreThanConcurrencyItems verifies that N > visionEnhancementConcurrency
// items are all processed without deadlock.
func TestVisionEnhancement_MoreThanConcurrencyItems(t *testing.T) {
	n := visionEnhancementConcurrency + 5
	swapVisionGlobals(t, fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			ans := "desc"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		fakePrompt,
	)

	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{
			"text":         fmt.Sprintf("item%d", i),
			"image":        "aGVsbG8=",
			"doc_type_kwd": "image",
		}
	}
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON:         items,
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("handled = false, want true")
	}
	// All n items should have received a description appended.
	for i, item := range res.JSON {
		got, _ := item["text"].(string)
		if got == fmt.Sprintf("item%d", i) {
			t.Errorf("item[%d] text unchanged %q, expected description appended", i, got)
		}
	}
}

// TestVisionEnhancement_PlainTextResponseNotTruncated verifies that a plain-text
// (no fences) VLM response is returned verbatim — common.CleanMarkdownBlock
// must not truncate it.
func TestVisionEnhancement_PlainTextResponseNotTruncated(t *testing.T) {
	plain := "A pipeline diagram showing three stages."
	swapVisionGlobals(t, fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			return &modelModule.ChatResponse{Answer: &plain}, nil
		},
		fakePrompt,
	)

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("handled = false, want true")
	}
	if got, _ := res.JSON[0]["text"].(string); got != plain {
		t.Errorf("plain text response = %q, want %q (must not be truncated)", got, plain)
	}
}

// TestVisionEnhancement_PromptBuilderErrorSkipped verifies that a prompt build
// failure skips enhancement silently (best-effort, nilerr is intentional).
func TestVisionEnhancement_PromptBuilderErrorSkipped(t *testing.T) {
	swapVisionGlobals(t,
		fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			t.Error("invoker should not be called when prompt builder fails")
			return nil, nil
		},
		func(_ string) (string, error) {
			return "", errors.New("prompt load failed")
		},
	)

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when prompt builder fails")
	}
	if got, _ := res.JSON[0]["text"].(string); got != "" {
		t.Errorf("text = %q, want empty", got)
	}
}

// TestVisionEnhancement_ModelResolveFailureSkipped verifies that a resolver error
// (e.g., tenant has no IMAGE2TEXT model) skips enhancement silently.
func TestVisionEnhancement_ModelResolveFailureSkipped(t *testing.T) {
	swapVisionGlobals(t,
		func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
			return nil, "", nil, 0, errors.New("no model")
		},
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			t.Error("invoker should not be called when model resolve fails")
			return nil, nil
		},
		fakePrompt,
	)

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when model resolve fails")
	}
	if got, _ := res.JSON[0]["text"].(string); got != "" {
		t.Errorf("text = %q, want empty", got)
	}
}

// TestVisionEnhancement_CancellationStopsSchedulingWithManyItems verifies that
// cancellation stops scheduling when N > visionEnhancementConcurrency, without deadlock.
func TestVisionEnhancement_CancellationStopsSchedulingWithManyItems(t *testing.T) {
	n := visionEnhancementConcurrency + 5
	swapVisionGlobals(t, fakeResolver,
		func(ctx context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			// Simulate work that respects ctx.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			ans := "desc"
			return &modelModule.ChatResponse{Answer: &ans}, nil
		},
		fakePrompt,
	)

	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{
			"text":         fmt.Sprintf("item%d", i),
			"image":        "aGVsbG8=",
			"doc_type_kwd": "image",
		}
	}
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON:         items,
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel — dispatch loop should break immediately

	res, handled, err := maybeDispatchVisionEnhancement(
		ctx,
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if handled {
		t.Error("handled = true with cancelled context, want false")
	}
	// No item should be modified when dispatch is cancelled before scheduling.
	for i, item := range res.JSON {
		if got, _ := item["text"].(string); got != fmt.Sprintf("item%d", i) {
			t.Errorf("item[%d] text = %q, want untouched after cancellation", i, got)
		}
	}
}

func TestBuildVisionMessages_PreventsDoublePrefix(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantURL string
	}{
		{
			name:    "raw base64 gets png prefix",
			input:   "aGVsbG8=",
			wantURL: "data:image/png;base64,aGVsbG8=",
		},
		{
			name:    "already png data uri is preserved without double prefix",
			input:   "data:image/png;base64,aGVsbG8=",
			wantURL: "data:image/png;base64,aGVsbG8=",
		},
		{
			name:    "jpeg data uri is preserved as jpeg",
			input:   "data:image/jpeg;base64,/9j/4AAQ",
			wantURL: "data:image/jpeg;base64,/9j/4AAQ",
		},
		{
			name:    "https url is preserved",
			input:   "https://example.com/figure.png",
			wantURL: "https://example.com/figure.png",
		},
		{
			name:    "whitespace trimmed",
			input:   "  data:image/png;base64,abc  ",
			wantURL: "data:image/png;base64,abc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := buildVisionMessages("describe", tc.input)
			if len(msgs) != 1 {
				t.Fatalf("len(msgs) = %d, want 1", len(msgs))
			}
			parts, ok := msgs[0].Content.([]interface{})
			if !ok || len(parts) < 2 {
				t.Fatalf("Content parts = %+v, want >= 2", msgs[0].Content)
			}
			img, ok := parts[1].(map[string]any)
			if !ok {
				t.Fatalf("parts[1] = %+v, want map[string]any", parts[1])
			}
			imgURL, ok := img["image_url"].(map[string]any)
			if !ok {
				t.Fatalf("img[image_url] = %+v, want map[string]any", img["image_url"])
			}
			got, _ := imgURL["url"].(string)
			if got != tc.wantURL {
				t.Errorf("image_url.url = %q, want %q", got, tc.wantURL)
			}
		})
	}
}

func TestExtractVisionAnswer_CleansMarkdownBlock(t *testing.T) {
	ans := "```markdown\n| Header 1 | Header 2 |\n| --- | --- |\n| Cell 1 | Cell 2 |\n```"
	resp := &modelModule.ChatResponse{Answer: &ans}
	got := extractVisionAnswer(resp)
	want := "| Header 1 | Header 2 |\n| --- | --- |\n| Cell 1 | Cell 2 |"
	if got != want {
		t.Errorf("extractVisionAnswer = %q, want %q", got, want)
	}
}

// TestExtractVisionAnswer_EdgeCases drives the shared
// common.CleanMarkdownBlock through the vision-answer path; the cleanup
// function itself is pinned in common.TestCleanMarkdownBlock.
func TestExtractVisionAnswer_EdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard markdown block",
			input: "```markdown\nHello world\n```",
			want:  "Hello world",
		},
		{
			name:  "windows CRLF endings",
			input: "```markdown\r\nHello world\r\n```",
			want:  "Hello world",
		},
		{
			name:  "nested code blocks preserved",
			input: "```markdown\nText with ```nested``` blocks\n```",
			want:  "Text with ```nested``` blocks",
		},
		{
			name:  "mixed whitespace tabs",
			input: "\t```markdown\t\n\tContent with tabs\n\t```\t",
			want:  "Content with tabs",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only markdown tags",
			input: "```markdown```",
			want:  "",
		},
		{
			name:  "unwrapped text unchanged",
			input: "Just plain text without markdown block",
			want:  "Just plain text without markdown block",
		},
		{
			name:  "unwrapped python block",
			input: "```python\nprint('hello')\n```",
			want:  "```python\nprint('hello')\n```",
		},
		{
			name:  "unwrapped JSON block",
			input: "```json\n{\"value\": 1}\n```",
			want:  "```json\n{\"value\": 1}\n```",
		},
		{
			name:  "unlabeled code block",
			input: "```\ncode without a language\n```",
			want:  "```\ncode without a language\n```",
		},
		{
			name:  "prose ending with code block",
			input: "Example:\n```python\nprint('hello')\n```",
			want:  "Example:\n```python\nprint('hello')\n```",
		},
		{
			name:  "multiple unwrapped code blocks",
			input: "```python\nfirst()\n```\n\n```python\nsecond()\n```",
			want:  "```python\nfirst()\n```\n\n```python\nsecond()\n```",
		},
		{
			name:  "unwrapped CRLF block",
			input: "  ```python\r\nprint('hello')\r\n```  ",
			want:  "```python\r\nprint('hello')\r\n```",
		},
		{
			name:  "closing fence without markdown opener",
			input: "Unopened block\n```",
			want:  "Unopened block\n```",
		},
		{
			name:  "markdown wrapper around code example",
			input: "```markdown\nExample:\n```python\nprint('hello')\n```\n```",
			want:  "Example:\n```python\nprint('hello')\n```",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &modelModule.ChatResponse{Answer: &tc.input}
			if got := extractVisionAnswer(resp); got != tc.want {
				t.Errorf("extractVisionAnswer(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPromptDirState_FailureIsSticky(t *testing.T) {
	dir := t.TempDir()
	var state promptDirState
	for i := 0; i < 2; i++ {
		if _, err := state.resolve(dir); err == nil {
			t.Fatalf("call %d: resolve() = nil error, want sticky init error", i+1)
		}
	}
}

func TestPromptDirState_SuccessIsCached(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rag", "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	var state promptDirState
	for i := 0; i < 2; i++ {
		got, err := state.resolve(dir)
		if err != nil {
			t.Fatalf("call %d: resolve() error: %v", i+1, err)
		}
		if got != dir {
			t.Fatalf("call %d: resolve() = %q, want %q", i+1, got, dir)
		}
	}
}

// TestVisionEnhancement_PerCallModelPreferred verifies that setups vlm.llm_id
// takes precedence over the tenant default IMAGE2TEXT model.
func TestVisionEnhancement_PerCallModelPreferred(t *testing.T) {
	origTenantResolver := resolveTenantModelByType
	origModelResolver := resolveModelConfig
	origInvoker := visionChatInvoker
	origPrompt := figureVisionPromptBuilder
	t.Cleanup(func() {
		resolveTenantModelByType = origTenantResolver
		resolveModelConfig = origModelResolver
		visionChatInvoker = origInvoker
		figureVisionPromptBuilder = origPrompt
	})

	tenantResolverCalled := false
	resolveTenantModelByType = func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		tenantResolverCalled = true
		return &visionEnhanceFakeDriver{}, "tenant-model", &modelModule.APIConfig{}, 0, nil
	}

	var gotRef string
	var gotType entity.ModelType
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, _ string, modelType entity.ModelType, ref string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		gotRef = ref
		gotType = modelType
		return &visionEnhanceFakeDriver{}, "custom-model", &modelModule.APIConfig{}, 0, nil
	}

	invoker := &visionEnhanceCaptureInvoker{}
	visionChatInvoker = invoker.invoke
	figureVisionPromptBuilder = fakePrompt

	setups := map[string]schema.ParserSetup{
		"pdf": {"vlm": map[string]any{"llm_id": "custom-vlm@provider"}},
	}
	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		},
	}

	_, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"},
		setups,
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if gotRef != "custom-vlm@provider" {
		t.Errorf("resolveModelConfig modelRef = %q, want %q", gotRef, "custom-vlm@provider")
	}
	if gotType != entity.ModelTypeImage2Text {
		t.Errorf("resolveModelConfig modelType = %q, want %q", gotType, entity.ModelTypeImage2Text)
	}
	if tenantResolverCalled {
		t.Error("tenant default resolver must not be called when setup vlm.llm_id is set")
	}
}

func TestVisionEnhancement_InvalidImageDataSkipped(t *testing.T) {
	invokerCalled := false
	swapVisionGlobals(t,
		fakeResolver,
		func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
			invokerCalled = true
			return nil, nil
		},
		fakePrompt,
	)

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "keep", "image": "!!!not-base64!!!", "doc_type_kwd": "image"},
		},
	}

	res, handled, err := maybeDispatchVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1"}, nil)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if handled {
		t.Error("handled = true, want false when invalid image data is skipped")
	}
	if invokerCalled {
		t.Error("invoker must not be called for invalid image data")
	}
	if got, _ := res.JSON[0]["text"].(string); got != "keep" {
		t.Errorf("text = %q, want unchanged %q", got, "keep")
	}
}

type deadlineCaptureDriver struct {
	modelModule.ModelDriver
	hasDeadline bool
	remaining   time.Duration
	config      *modelModule.ChatConfig
}

func (d *deadlineCaptureDriver) ChatWithMessages(
	ctx context.Context,
	_ string,
	_ []modelModule.Message,
	_ *modelModule.APIConfig,
	config *modelModule.ChatConfig,
	_ *common.ModelUsage,
) (*modelModule.ChatResponse, error) {
	deadline, ok := ctx.Deadline()
	d.hasDeadline = ok
	d.config = config
	if ok {
		d.remaining = time.Until(deadline)
	}
	ans := "ok"
	return &modelModule.ChatResponse{Answer: &ans}, nil
}

func TestDefaultVisionChatInvoker_AppliesDeadline(t *testing.T) {
	drv := &deadlineCaptureDriver{}
	if _, err := defaultVisionChatInvoker(context.Background(), drv, "m", nil, nil); err != nil {
		t.Fatalf("defaultVisionChatInvoker: %v", err)
	}
	if !drv.hasDeadline {
		t.Fatal("vision chat context must have a deadline")
	}
	if drv.remaining <= 0 || drv.remaining > visionChatTimeout+time.Second {
		t.Fatalf("deadline remaining = %v, want ~%v", drv.remaining, visionChatTimeout)
	}
	if drv.config == nil || drv.config.Vision == nil || !*drv.config.Vision {
		t.Fatal("vision chat must enable vision")
	}
	if drv.config.Thinking != nil {
		t.Fatal("non-Ollama vision chat must preserve provider thinking default")
	}
}
