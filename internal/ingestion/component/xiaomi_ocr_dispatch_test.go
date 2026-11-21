//go:build cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

func TestXiaomiOCRPDFSelectedPage(t *testing.T) {
	db, composite, _ := tenantPDFParserFixture(t, "XiaomiOCR")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-key-b" {
			t.Errorf("unexpected endpoint or authentication: %s", r.URL.Path)
		}
		var req struct {
			Model    string
			Messages []struct {
				Role    string
				Content []struct {
					Type, Text string
					Image      struct{ URL string } `json:"image_url"`
				}
			}
			Template map[string]bool `json:"chat_template_kwargs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		thinking, ok := req.Template["enable_thinking"]
		if req.Model != "XiaomiOCR-model" || !ok || thinking || len(req.Messages) != 1 || req.Messages[0].Role != "user" || len(req.Messages[0].Content) != 2 {
			t.Error("invalid OCR request")
			return
		}
		content := req.Messages[0].Content
		b64, ok := strings.CutPrefix(content[0].Image.URL, "data:image/png;base64,")
		if !ok || content[0].Type != "image_url" || content[1].Type != "text" || !strings.Contains(content[1].Text, "OTSL") {
			t.Error("missing image or whole-page prompt")
			return
		}
		img, err := png.DecodeConfig(base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64)))
		if err != nil || img.Width < 1 || img.Height < 1 || max(img.Width, img.Height) > 2048 {
			t.Errorf("bad raster size: %+v, %v", img, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"# Analysis\n\nMass is \\(mc^2\\).\n\n<fcel>A<fcel>B<nl><fcel>10<fcel>20<nl>"}}]}`))
	}))
	defer server.Close()
	extra, _ := json.Marshal(map[string]string{"base_url": server.URL + "/v1"})
	if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-b").Update("extra", string(extra)).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ field, ref string }{{"parse_method", "model-b"}, {"layout_recognizer", composite}} {
		t.Run(tc.field, func(t *testing.T) {
			setup := schema.ParserSetup{"pages": []any{[]any{2, 2}}, tc.field: tc.ref}
			res, handled, err := maybeDispatchPDFVision(t.Context(), db, utility.FileTypePDF, "fixture.pdf", makeTestPDF(3), map[string]any{"tenant_id": "tenant-1"}, map[string]schema.ParserSetup{"pdf": setup})
			if err != nil || !handled || res.Err != nil {
				t.Fatalf("handled=%v, err=%v, result=%v", handled, err, res.Err)
			}
			var heading, formula, table bool
			for _, item := range res.JSON {
				fields := item.ToMap()
				if fields["page_number"] != 2 {
					t.Errorf("wrong source page: %v", fields)
				}
				text, _ := fields["text"].(string)
				heading = heading || strings.Contains(text, "Analysis")
				formula = formula || strings.Contains(text, "mc^2")
				if td, ok := fields["table"].(*entity.TableData); ok && td != nil {
					data, _ := json.Marshal(td)
					table = td.Page == 2 && strings.Contains(string(data), "20")
				}
			}
			if !heading || !formula || !table || res.File["page_count"] != 3 {
				t.Fatalf("unexpected result: %+v, %v", res.JSON, res.File)
			}
		})
	}
	if calls.Load() != 2 {
		t.Fatalf("sent %d pages, want one per invocation", calls.Load())
	}
}
