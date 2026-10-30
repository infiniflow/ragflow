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

package models

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func withFastMiniCPMOCRPolling(t *testing.T) {
	t.Helper()
	prev := miniCPMOCRPollInterval
	miniCPMOCRPollInterval = time.Millisecond
	t.Cleanup(func() { miniCPMOCRPollInterval = prev })
}

func newMiniCPMOCRForTest(baseURL string) *MiniCPMOCRModel {
	return NewMiniCPMOCRModel(map[string]string{"default": baseURL}, URLSuffix{OCR: "document-parse/jobs"})
}

func miniCPMOCRCall(t *testing.T, driver *MiniCPMOCRModel, content []byte, url *string) (*OCRFileResponse, error) {
	t.Helper()
	model, key := "MiniCPM-OCR-1.0", "test-key"
	return driver.OCRFile(t.Context(), &model, content, url, &APIConfig{ApiKey: &key}, nil, nil)
}

// The shipped config must resolve to the MiniCPM-OCR driver: CreateModelDriver
// dispatches on the lower-cased provider name, so a mismatch would silently
// fall through to DummyModel.
func TestMiniCPMOCRProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "minicpm_ocr.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}
	provider := GetProviderManager().FindProvider("MiniCPM-OCR")
	if provider == nil {
		t.Fatal("MiniCPM-OCR provider not found")
	}
	driver, ok := provider.ModelDriver.(*MiniCPMOCRModel)
	if !ok {
		t.Fatalf("ModelDriver=%T, want *MiniCPMOCRModel", provider.ModelDriver)
	}
	if driver.Name() != "minicpm-ocr" {
		t.Fatalf("Name()=%q", driver.Name())
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*MiniCPMOCRModel); !ok {
		t.Fatal("NewInstance did not return *MiniCPMOCRModel")
	}
	if len(provider.Models) != 1 || provider.Models[0].Name != "MiniCPM-OCR-1.0" || !provider.Models[0].ModelTypeMap["ocr"] {
		t.Fatalf("models=%+v", provider.Models)
	}
}

// OCRFile uploads the document with the job request, polls until the job
// succeeds and keeps every page's blocks in reading order.
func TestMiniCPMOCRFileRunsJob(t *testing.T) {
	withSSRFBypass(t)
	withFastMiniCPMOCRPolling(t)

	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/document-parse/jobs":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm: %v", err)
			}
			if got := r.FormValue("model"); got != "MiniCPM-OCR-1.0" {
				t.Errorf("model=%q", got)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("FormFile: %v", err)
			}
			data, _ := io.ReadAll(file)
			if header.Filename != "document.pdf" || string(data) != "%PDF-1.7 test" {
				t.Errorf("file=%q content=%q", header.Filename, data)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"ocr_1","status":"Running","pages_done":0,"pages_total":0}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/document-parse/jobs/ocr_1":
			if polls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"id":"ocr_1","status":"Running","pages_done":0,"pages_total":2,"pages":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"id": "ocr_1", "status": "Success", "pages_done": 2, "pages_total": 2,
				"pages": [
					{"metadata": {"page_index": 1},
					 "choices": [{"message": {"content": "Second page."}}],
					 "ocr": {"image_width": 1240, "image_height": 1754, "blocks": [
						{"order": 1, "label": "text", "bbox": [90, 100, 1100, 160], "regions": [[90, 100, 1100, 160]], "content": "Second page.", "status": "ok"}]}},
					{"metadata": {"page_index": 0},
					 "choices": [{"message": {"content": "# Title\n\nBody text."}}],
					 "ocr": {"image_width": 1240, "image_height": 1754, "blocks": [
						{"order": 3, "label": "image", "bbox": [100, 400, 900, 800], "content": "", "status": "skipped_no_vlm"},
						{"order": 2, "label": "text", "bbox": [100, 200, 1000, 260], "content": "Body text.", "status": "ok"},
						{"order": 0, "label": "header", "bbox": [100, 20, 600, 50], "content": "Running header", "status": "ok"},
						{"order": 1, "label": "doc_title", "bbox": [100, 80, 800, 150], "content": "Title", "status": "ok"},
						{"order": 4, "label": "table", "bbox": [100, 900, 1100, 1200], "content": "", "status": "failed"},
						{"order": 5, "label": "number", "bbox": [600, 1700, 640, 1730], "content": "1", "status": "ok"},
						{"order": 6, "label": "footer_image", "bbox": [1000, 1700, 1100, 1740], "content": "", "status": "skipped_no_vlm"}]}}
				],
				"markdown": "# Title\n\nBody text.\n\nSecond page."
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	response, err := miniCPMOCRCall(t, newMiniCPMOCRForTest(server.URL+"/v1"), []byte("%PDF-1.7 test"), nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if got := *response.Text; got != "# Title\n\nBody text.\n\nSecond page." {
		t.Fatalf("text=%q", got)
	}
	if len(response.Pages) != 2 || response.Pages[0].Index != 0 || response.Pages[1].Index != 1 {
		t.Fatalf("pages=%+v", response.Pages)
	}
	first := response.Pages[0]
	if first.Width != 1240 || first.Height != 1754 {
		t.Fatalf("page size=%vx%v", first.Width, first.Height)
	}
	want := []OCRBlock{
		{Text: "Title", Label: "doc_title", BBox: []float64{100, 80, 800, 150}},
		{Text: "Body text.", Label: "text", BBox: []float64{100, 200, 1000, 260}},
		{Text: "", Label: "image", BBox: []float64{100, 400, 900, 800}},
		{Text: "", Label: "table", BBox: []float64{100, 900, 1100, 1200}},
	}
	if !reflect.DeepEqual(first.Blocks, want) {
		t.Fatalf("blocks=%+v\nwant %+v", first.Blocks, want)
	}
	if len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], "page 1: table block 4") {
		t.Fatalf("warnings=%q", response.Warnings)
	}
	if polls.Load() != 2 {
		t.Fatalf("polls=%d, want 2", polls.Load())
	}
}

// A page the API turned upright for recognition keeps its text but loses its
// boxes, which locate content on the turned page; a deskewed page keeps them.
// The rotation records follow a hosted response for an upright, an
// upside-down and a sideways copy of one scanned page.
func TestMiniCPMOCRTurnedPagesKeepTextWithoutCoordinates(t *testing.T) {
	var job miniCPMOCRJob
	if err := json.Unmarshal([]byte(`{"id": "ocr_7", "status": "Success", "pages": [
		{"metadata": {"page_index": 0}, "choices": [{"message": {"content": "Upright form."}}],
		 "ocr": {"image_width": 748, "image_height": 972,
			"rotation": {"applied": true, "enabled": true, "total_angle": -1.0, "deskew_angle": -1.0, "orientation_angle": 0},
			"blocks": [{"order": 0, "label": "doc_title", "bbox": [256, 199, 422, 227], "content": "Upright form.", "status": "ok"}]}},
		{"metadata": {"page_index": 1}, "choices": [{"message": {"content": "Upside-down form."}}],
		 "ocr": {"image_width": 748, "image_height": 972,
			"rotation": {"applied": true, "enabled": true, "total_angle": 179.0, "deskew_angle": -1.0, "orientation_angle": 180},
			"blocks": [{"order": 0, "label": "doc_title", "bbox": [256, 198, 422, 226], "content": "Upside-down form.", "status": "ok"}]}},
		{"metadata": {"page_index": 2}, "choices": [{"message": {"content": "Sideways form."}}],
		 "ocr": {"image_width": 748, "image_height": 972,
			"rotation": {"applied": true, "enabled": true, "total_angle": 269.0, "deskew_angle": -1.0, "orientation_angle": 270},
			"blocks": [{"order": 0, "label": "doc_title", "bbox": [256, 199, 422, 227], "content": "Sideways form.", "status": "ok"}]}}
	]}`), &job); err != nil {
		t.Fatal(err)
	}

	response := job.response()
	if *response.Text != "Upright form.\n\nUpside-down form.\n\nSideways form." || len(response.Pages) != 3 {
		t.Fatalf("response=%+v", response)
	}
	upright := response.Pages[0]
	if upright.Width != 748 || upright.Height != 972 || !reflect.DeepEqual(upright.Blocks[0].BBox, []float64{256, 199, 422, 227}) {
		t.Fatalf("deskewed page lost its coordinates: %+v", upright)
	}
	for _, page := range response.Pages[1:] {
		if page.Width != 0 || page.Height != 0 || len(page.Blocks) != 1 || page.Blocks[0].BBox != nil || page.Blocks[0].Text == "" {
			t.Fatalf("turned page %d: %+v", page.Index, page)
		}
	}
	if len(response.Warnings) != 2 || !strings.Contains(response.Warnings[0], "page 2 was turned 180 degrees") || !strings.Contains(response.Warnings[1], "page 3 was turned 270 degrees") {
		t.Fatalf("warnings=%q", response.Warnings)
	}
}

// A document reachable at a public URL is referenced in a JSON body instead
// of being uploaded.
func TestMiniCPMOCRFileSubmitsURL(t *testing.T) {
	withSSRFBypass(t)
	withFastMiniCPMOCRPolling(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode: %v", err)
			}
			if body["model"] != "MiniCPM-OCR-1.0" || body["url"] != "https://example.com/report.pdf" {
				t.Errorf("body=%v", body)
			}
			w.WriteHeader(http.StatusAccepted)
		}
		_, _ = w.Write([]byte(`{"id":"ocr_2","status":"Success","pages":[{"metadata":{"page_index":0},"choices":[{"message":{"content":"Remote."}}],"ocr":{"blocks":[]}}]}`))
	}))
	defer server.Close()

	url := "https://example.com/report.pdf"
	response, err := miniCPMOCRCall(t, newMiniCPMOCRForTest(server.URL), nil, &url)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if *response.Text != "Remote." {
		t.Fatalf("text=%q", *response.Text)
	}
}

// An image without text still completes, so the provider check that sends a
// blank image succeeds.
func TestMiniCPMOCRFileAcceptsEmptyResult(t *testing.T) {
	withSSRFBypass(t)
	withFastMiniCPMOCRPolling(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = r.ParseMultipartForm(1 << 20)
			if _, header, err := r.FormFile("file"); err != nil || header.Filename != "document.png" {
				t.Errorf("file=%v err=%v", header, err)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"ocr_3","status":"Running"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"ocr_3","status":"Success","pages":[{"metadata":{"page_index":0},"choices":[{"message":{"content":""}}],"ocr":{"image_width":1,"image_height":1,"blocks":[]}}]}`))
	}))
	defer server.Close()

	response, err := miniCPMOCRCall(t, newMiniCPMOCRForTest(server.URL), []byte("\x89PNG\r\n\x1a\nblank"), nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if response.Text == nil || *response.Text != "" || len(response.Pages) != 1 {
		t.Fatalf("response=%+v", response)
	}
}

func TestMiniCPMOCRFileReportsJobFailure(t *testing.T) {
	withSSRFBypass(t)
	withFastMiniCPMOCRPolling(t)

	for name, errorField := range map[string]string{
		"string": `"imageParseInvalidResult"`,
		"object": `{"message":"imageParseInvalidResult","code":"invalid"}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"id":"ocr_4","status":"Running"}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":"ocr_4","status":"Failed","pages_done":0,"pages_total":1,"error":` + errorField + `}`))
			}))
			defer server.Close()

			_, err := miniCPMOCRCall(t, newMiniCPMOCRForTest(server.URL), []byte("%PDF-1.7"), nil)
			if err == nil || !strings.Contains(err.Error(), "job ocr_4 failed: imageParseInvalidResult") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestMiniCPMOCRFileReportsHTTPError(t *testing.T) {
	withSSRFBypass(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"invalid_request_error"}}`))
	}))
	defer server.Close()

	_, err := miniCPMOCRCall(t, newMiniCPMOCRForTest(server.URL), []byte("%PDF-1.7"), nil)
	var statusErr *APIStatusError
	if !errors.As(err, &statusErr) || statusErr.Status != http.StatusUnauthorized || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("err=%v", err)
	}
}

// A run that stops waiting cancels the job on the server, which limits how
// many jobs an account may keep active.
func TestMiniCPMOCRFileCancelsAbandonedJob(t *testing.T) {
	withSSRFBypass(t)
	withFastMiniCPMOCRPolling(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"ocr_5","status":"Running"}`))
		case http.MethodGet:
			cancel()
			_, _ = w.Write([]byte(`{"id":"ocr_5","status":"Running"}`))
		case http.MethodDelete:
			if r.URL.Path != "/document-parse/jobs/ocr_5" || r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("cancel path=%q auth=%q", r.URL.Path, r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"id":"ocr_5","status":"Cancelled"}`))
			close(cancelled)
		}
	}))
	defer server.Close()

	model, key := "MiniCPM-OCR-1.0", "test-key"
	_, err := newMiniCPMOCRForTest(server.URL).OCRFile(ctx, &model, []byte("%PDF-1.7"), nil, &APIConfig{ApiKey: &key}, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("job was not cancelled on the server")
	}
}

// Polling retries rate limiting and server errors; a job the driver gives up
// on is cancelled on the server.
func TestMiniCPMOCRFilePollFailures(t *testing.T) {
	withSSRFBypass(t)
	withFastMiniCPMOCRPolling(t)

	for _, tc := range []struct {
		name       string
		failures   int
		status     int
		wantErr    bool
		wantCancel bool
	}{
		{name: "transient failure is retried", failures: 2, status: http.StatusServiceUnavailable},
		{name: "persistent server errors give up", failures: 100, status: http.StatusBadGateway, wantErr: true, wantCancel: true},
		{name: "client error gives up at once", failures: 1, status: http.StatusNotFound, wantErr: true, wantCancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var polls, cancels atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodPost:
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"id":"ocr_6","status":"Running"}`))
				case http.MethodGet:
					if int(polls.Add(1)) <= tc.failures {
						w.WriteHeader(tc.status)
						_, _ = w.Write([]byte(`{"error":{"message":"unavailable"}}`))
						return
					}
					_, _ = w.Write([]byte(`{"id":"ocr_6","status":"Success","pages":[{"metadata":{"page_index":0},"choices":[{"message":{"content":"Recovered."}}],"ocr":{"blocks":[]}}]}`))
				case http.MethodDelete:
					cancels.Add(1)
					_, _ = w.Write([]byte(`{"id":"ocr_6","status":"Cancelled"}`))
				}
			}))
			defer server.Close()

			response, err := miniCPMOCRCall(t, newMiniCPMOCRForTest(server.URL), []byte("%PDF-1.7"), nil)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err=%v", err)
			}
			if !tc.wantErr && *response.Text != "Recovered." {
				t.Fatalf("text=%q", *response.Text)
			}
			if got := cancels.Load() > 0; got != tc.wantCancel {
				t.Fatalf("cancelled=%v, want %v", got, tc.wantCancel)
			}
		})
	}
}

func TestMiniCPMOCRFileRequiresAPIKeyAndInput(t *testing.T) {
	driver := newMiniCPMOCRForTest("http://unused")
	model, empty, key := "MiniCPM-OCR-1.0", "", "test-key"
	if _, err := driver.OCRFile(t.Context(), &model, []byte("%PDF-1.7"), nil, &APIConfig{ApiKey: &empty}, nil, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("missing key err=%v", err)
	}
	if _, err := driver.OCRFile(t.Context(), &model, nil, nil, &APIConfig{ApiKey: &key}, nil, nil); err == nil || !strings.Contains(err.Error(), "file url or content is required") {
		t.Fatalf("missing input err=%v", err)
	}
}
