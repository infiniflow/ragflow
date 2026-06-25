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

package deepdoc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastBackoff returns a Client that uses a 1ms backoff so retry-loop
// tests finish in milliseconds rather than the production 200ms
// default. Keeps the assertions tight without giving up the real
// retry semantics.
func fastBackoff() Option { return WithBackoff(1 * time.Millisecond) }

// readMultipart parses the request body and returns the file
// uploaded under the "request" field plus the detected
// content-type. The DLA server expects a single image/jpeg part
// named "request" (deepdoc/vision/dla_cli.py:25-40). Returns
// errors instead of using t.Fatal so the recordingServer handler
// (which has no *testing.T) can call it without NPEs.
func readMultipart(r *http.Request) (fileBytes []byte, contentType string, err error) {
	if err = r.ParseMultipartForm(1 << 20); err != nil {
		return nil, "", err
	}
	fh, ok := r.MultipartForm.File["request"]
	if !ok || len(fh) == 0 {
		keys := make([]string, 0, len(r.MultipartForm.File))
		for k := range r.MultipartForm.File {
			keys = append(keys, k)
		}
		return nil, "", &multipartFieldError{Field: "request", Keys: keys}
	}
	f, err := fh[0].Open()
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	fileBytes, err = io.ReadAll(f)
	if err != nil {
		return nil, "", err
	}
	contentType = fh[0].Header.Get("Content-Type")
	return
}

// multipartFieldError is returned by readMultipart when the
// expected "request" field is missing — the test helpers turn
// it into a t.Fatalf with a useful message.
type multipartFieldError struct {
	Field string
	Keys  []string
}

func (e *multipartFieldError) Error() string {
	return "missing '" + e.Field + "' field; got keys=" + strings.Join(e.Keys, ",")
}

// recordingServer is a tiny wrapper that lets a test count requests
// and stage the responses the retry loop should observe.
type recordingServer struct {
	requests int64
	calls    []recordedCall
	handler  func(w http.ResponseWriter, r *http.Request, call int)
}

type recordedCall struct {
	body        []byte
	contentType string
}

func (s *recordingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := atomic.AddInt64(&s.requests, 1)
	// The recording handler deliberately ignores readMultipart
	// errors — callers that care about the body assert it
	// themselves in their own http.HandlerFunc.
	body, ct, _ := readMultipart(r)
	s.calls = append(s.calls, recordedCall{body: body, contentType: ct})
	s.handler(w, r, int(n))
}

func TestDLA_SuccessfulResponse(t *testing.T) {
	srv := httptest.NewServer(&recordingServer{
		handler: func(w http.ResponseWriter, r *http.Request, call int) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bboxes": [][]float64{
					{10, 20, 100, 200, 0.95, 0}, // title
					{10, 220, 500, 400, 0.88, 1}, // text
					{50, 420, 600, 700, 0.77, 3}, // figure
				},
			})
		},
	})
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff())
	res, err := c.DLA(context.Background(), [][]byte{[]byte("jpg-bytes")})
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("len(res)=%d, want 3", len(res))
	}
	wantTypes := []string{"title", "text", "figure"}
	wantScores := []float64{0.95, 0.88, 0.77}
	wantBBox := []BBox{{10, 20, 100, 200}, {10, 220, 500, 400}, {50, 420, 600, 700}}
	for i, r := range res {
		if r.Type != wantTypes[i] {
			t.Errorf("[%d] Type=%q, want %q", i, r.Type, wantTypes[i])
		}
		if r.Score != wantScores[i] {
			t.Errorf("[%d] Score=%v, want %v", i, r.Score, wantScores[i])
		}
		if r.BBox != wantBBox[i] {
			t.Errorf("[%d] BBox=%v, want %v", i, r.BBox, wantBBox[i])
		}
	}
	// TypeIdx is preserved for downstream callers that care about
	// the duplicate-class disambiguation.
	if res[0].TypeIdx != 0 {
		t.Errorf("res[0].TypeIdx=%d, want 0", res[0].TypeIdx)
	}
	if res[1].TypeIdx != 1 {
		t.Errorf("res[1].TypeIdx=%d, want 1", res[1].TypeIdx)
	}
}

func TestDLA_MultipartFieldNameAndContentType(t *testing.T) {
	var gotReqCT, gotPartCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bytes, ct, err := readMultipart(r)
		if err != nil {
			t.Fatalf("readMultipart: %v", err)
		}
		gotReqCT = r.Header.Get("Content-Type")
		gotPartCT = ct
		gotBody = string(bytes)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bboxes":[]}`))
	}))
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff())
	if _, err := c.DLA(context.Background(), [][]byte{[]byte("PAYLOAD")}); err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if gotBody != "PAYLOAD" {
		t.Errorf("body=%q, want PAYLOAD (DLA must round-trip the JPEG bytes unchanged)", gotBody)
	}
	if !strings.HasPrefix(gotReqCT, "multipart/form-data") {
		t.Errorf("request content-type=%q, want multipart/form-data (set by multipart.Writer.FormDataContentType)", gotReqCT)
	}
	// The part's own content-type is image/jpeg, asserted via
	// readMultipart returning it; we already covered the boundary
	// case in TestDLA_PartContentType. We just record it here as a
	// sanity check.
	if gotPartCT != "image/jpeg" {
		t.Errorf("part content-type=%q, want image/jpeg", gotPartCT)
	}
}

func TestDLA_PartContentType(t *testing.T) {
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ct, err := readMultipart(r)
		if err != nil {
			t.Fatalf("readMultipart: %v", err)
		}
		gotCT = ct
		_, _ = w.Write([]byte(`{"bboxes":[]}`))
	}))
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff())
	if _, err := c.DLA(context.Background(), [][]byte{[]byte("img")}); err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if gotCT != "image/jpeg" {
		t.Errorf("part Content-Type=%q, want image/jpeg (matches dla_cli.py:35)", gotCT)
	}
}

func TestDLA_TrimsTrailingSlash(t *testing.T) {
	var hit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		_, _ = w.Write([]byte(`{"bboxes":[]}`))
	}))
	defer srv.Close()

	c := NewClientWithURL(srv.URL+"/", fastBackoff())
	if _, err := c.DLA(context.Background(), [][]byte{[]byte("img")}); err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if hit != "/predict" {
		t.Errorf("path=%q, want /predict (no double slash)", hit)
	}
}

func TestDLA_RetriesOn5xxThenSucceeds(t *testing.T) {
	srv := httptest.NewServer(&recordingServer{
		handler: func(w http.ResponseWriter, r *http.Request, call int) {
			if call < 2 {
				http.Error(w, "transient", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"bboxes":[[0,0,1,1,0.5,5]]}`))
		},
	})
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff(), WithMaxAttempts(3))
	res, err := c.DLA(context.Background(), [][]byte{[]byte("img")})
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(res) != 1 || res[0].Type != "table" {
		t.Errorf("res=%+v, want one 'table' result after 2nd-attempt success", res)
	}
}

func TestDLA_ExhaustsRetriesOnPersistent5xx(t *testing.T) {
	srv := httptest.NewServer(&recordingServer{
		handler: func(w http.ResponseWriter, r *http.Request, call int) {
			http.Error(w, "down", http.StatusServiceUnavailable)
		},
	})
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff(), WithMaxAttempts(3))
	res, err := c.DLA(context.Background(), [][]byte{[]byte("img")})
	if err != nil {
		t.Fatalf("DLA error=%v, want nil (per-image failures map to empty slot)", err)
	}
	if len(res) != 1 {
		t.Fatalf("len(res)=%d, want 1 (failed image yields empty slot)", len(res))
	}
	if res[0].Type != "" || res[0].Score != 0 {
		t.Errorf("res[0]=%+v, want zero-value DLAResult for failed image", res[0])
	}
}

func TestDLA_4xxReturnsEmpty(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		http.Error(w, "bad image", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff(), WithMaxAttempts(5))
	res, err := c.DLA(context.Background(), [][]byte{[]byte("img")})
	if err != nil {
		t.Fatalf("DLA error=%v, want nil", err)
	}
	if len(res) != 1 || res[0].Type != "" {
		t.Errorf("res=%+v, want single empty slot", res)
	}
	// 4xx is not retried.
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("calls=%d, want 1 (4xx is a config error, not transient)", got)
	}
}

func TestDLA_RetriesOnMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(&recordingServer{
		handler: func(w http.ResponseWriter, r *http.Request, call int) {
			if call < 2 {
				_, _ = w.Write([]byte(`<<< not json`))
				return
			}
			_, _ = w.Write([]byte(`{"bboxes":[[0,0,1,1,0.5,2]]}`))
		},
	})
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff(), WithMaxAttempts(3))
	res, err := c.DLA(context.Background(), [][]byte{[]byte("img")})
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(res) != 1 || res[0].Type != "reference" {
		t.Errorf("res=%+v, want one 'reference' after retry", res)
	}
}

func TestDLA_RetriesOnMissingBBoxes(t *testing.T) {
	srv := httptest.NewServer(&recordingServer{
		handler: func(w http.ResponseWriter, r *http.Request, call int) {
			if call < 2 {
				_, _ = w.Write([]byte(`{"wrong":"key"}`))
				return
			}
			_, _ = w.Write([]byte(`{"bboxes":[]}`))
		},
	})
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff(), WithMaxAttempts(3))
	res, err := c.DLA(context.Background(), [][]byte{[]byte("img")})
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(res) != 1 {
		t.Errorf("len(res)=%d, want 1 (empty bboxes is success)", len(res))
	}
}

func TestDLA_PerImageIsolation(t *testing.T) {
	// First image: 503 on both attempts (exhausts with
	// MaxAttempts=2). Second image: 200. Verifies that a failed
	// image does not abort the batch (matches the Python
	// "len(res) == i" append-empty pattern).
	srv := httptest.NewServer(&recordingServer{
		handler: func(w http.ResponseWriter, r *http.Request, call int) {
			if call < 3 {
				http.Error(w, "boom", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"bboxes":[[0,0,10,10,0.9,1]]}`))
		},
	})
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff(), WithMaxAttempts(2))
	res, err := c.DLA(context.Background(), [][]byte{[]byte("a"), []byte("b")})
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("len(res)=%d, want 2", len(res))
	}
	if res[0].Type != "" {
		t.Errorf("res[0]=%+v, want empty (first image failed)", res[0])
	}
	if res[1].Type != "text" {
		t.Errorf("res[1]=%+v, want text (second image succeeded)", res[1])
	}
}

func TestDLA_SkipsShortBBoxes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// One well-formed bbox, one too-short, one well-formed.
		_, _ = w.Write([]byte(`{"bboxes":[[0,0,1,1,0.5,1],[1,2,3], [0,0,1,1,0.5,3]]}`))
	}))
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff())
	res, err := c.DLA(context.Background(), [][]byte{[]byte("img")})
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("len(res)=%d, want 2 (short bbox must be skipped, not panic)", len(res))
	}
	if res[0].Type != "text" || res[1].Type != "figure" {
		t.Errorf("types=%q,%q, want text,figure", res[0].Type, res[1].Type)
	}
}

func TestDLA_OutOfRangeTypeIdxMapsToEmptyString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TypeIdx 42 is beyond DLAClasses (len 10) — must not panic.
		_, _ = w.Write([]byte(`{"bboxes":[[0,0,1,1,0.5,42]]}`))
	}))
	defer srv.Close()

	c := NewClientWithURL(srv.URL, fastBackoff())
	res, err := c.DLA(context.Background(), [][]byte{[]byte("img")})
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("len(res)=%d, want 1", len(res))
	}
	if res[0].Type != "" {
		t.Errorf("Type=%q, want empty string for out-of-range TypeIdx", res[0].Type)
	}
	if res[0].TypeIdx != 42 {
		t.Errorf("TypeIdx=%d, want 42 (raw value preserved)", res[0].TypeIdx)
	}
}

func TestDLA_ContextCancelDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// 5s backoff × 5 attempts = 25s of pure sleep if cancel is
	// ignored. Assert the call returns in well under that — the
	// meaningful property here is "ctx cancel short-circuits the
	// retry loop", not the specific error value, because DLA
	// collapses per-image failures into empty slots by design
	// (matches the Python contract from layout_recognizer.py:74-76).
	c := NewClientWithURL(srv.URL, WithBackoff(5*time.Second), WithMaxAttempts(5))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := c.DLA(ctx, [][]byte{[]byte("img")})
	elapsed := time.Since(start)
	if err != nil {
		t.Logf("DLA err=%v (acceptable)", err)
	}
	// Allow generous slack for CI scheduling jitter — 2s is still
	// 12× shorter than the unsuppressed 25s total backoff.
	if elapsed > 2*time.Second {
		t.Errorf("DLA took %v with ctx cancelled at 50ms; retry loop ignored cancel", elapsed)
	}
}
