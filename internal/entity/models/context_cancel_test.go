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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDriverRequestsHonourContextCancellation pins that a cancelled run also
// aborts the upstream HTTP call. A request built with http.NewRequest instead
// of NewRequestWithContext keeps the connection open until the client timeout
// even after the caller gave up, so a cancelled ingest or chat keeps billing
// the provider. Drop a site from that pattern and its subtest hangs until the
// server's bounded wait expires.
func TestDriverRequestsHonourContextCancellation(t *testing.T) {
	withSSRFBypass(t)

	// The handler waits for the client to cancel its own request context, but
	// bounded so a request that ignores cancellation cannot hang the suite.
	// release lets cleanup end the handler immediately instead of waiting out
	// the bound.
	blocking := func(t *testing.T) *httptest.Server {
		t.Helper()
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-release:
			case <-time.After(500 * time.Millisecond):
			}
		}))
		t.Cleanup(srv.Close)
		t.Cleanup(func() { close(release) })
		return srv
	}
	run := func(t *testing.T, call func(context.Context) error) {
		t.Helper()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		time.AfterFunc(50*time.Millisecond, cancel)
		start := time.Now()
		err := call(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
			t.Fatalf("call returned after %v; the request ignored the cancelled context", elapsed)
		}
	}
	apiConfig := func() *APIConfig {
		return &APIConfig{ApiKey: ptr("test-key")}
	}

	t.Run("builtin embed", func(t *testing.T) {
		srv := blocking(t)
		m := NewBuiltinModel(srv.URL, "m")
		run(t, func(ctx context.Context) error {
			_, err := m.Embed(ctx, ptr("m"), EmbedRequest{Texts: []string{"hello"}}, apiConfig(), nil, nil)
			return err
		})
	})

	t.Run("xai transcribe audio", func(t *testing.T) {
		srv := blocking(t)
		m := NewXAIModel(map[string]string{"default": srv.URL}, URLSuffix{ASR: "asr"})
		audio := filepath.Join(t.TempDir(), "clip.mp3")
		if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
			t.Fatalf("write audio fixture: %v", err)
		}
		run(t, func(ctx context.Context) error {
			_, err := m.TranscribeAudio(ctx, ptr("m"), ptr(audio), apiConfig(), nil, nil)
			return err
		})
	})

	t.Run("groq audio speech", func(t *testing.T) {
		srv := blocking(t)
		m := NewGroqModel(map[string]string{"default": srv.URL}, URLSuffix{TTS: "tts"})
		run(t, func(ctx context.Context) error {
			_, err := m.AudioSpeech(ctx, ptr("voice"), ptr("hello"), apiConfig(), nil, nil)
			return err
		})
	})

	t.Run("togetherai rerank", func(t *testing.T) {
		srv := blocking(t)
		m := NewTogetherAIModel(map[string]string{"default": srv.URL}, URLSuffix{Rerank: "rerank"})
		run(t, func(ctx context.Context) error {
			_, err := m.Rerank(ctx, ptr("m"), RerankRequest{Query: "q", Documents: []string{"a"}}, apiConfig(), &RerankConfig{}, nil)
			return err
		})
	})

	t.Run("jina list models", func(t *testing.T) {
		srv := blocking(t)
		m := NewJinaModel(map[string]string{"default": srv.URL}, URLSuffix{Models: "models"})
		run(t, func(ctx context.Context) error {
			_, err := m.ListModels(ctx, apiConfig())
			return err
		})
	})

	t.Run("novita balance", func(t *testing.T) {
		srv := blocking(t)
		m := NewNovitaModel(map[string]string{"default": srv.URL}, URLSuffix{Balance: "balance"})
		run(t, func(ctx context.Context) error {
			_, err := m.Balance(ctx, apiConfig())
			return err
		})
	})

	// A non-200 body that fails mid-read must report that failure. Discarding the
	// read error turns an interrupted body (or a cancellation) into a bare status
	// line, which hides why the retry-worthy call actually failed.
	truncatedError := func(t *testing.T) *httptest.Server {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			defer conn.Close()
			// Promise a body and send none: the first read fails, so a driver that
			// swallows the read error can only report the status line.
			buf.WriteString("HTTP/1.1 500 Internal Server Error\r\nContent-Length: 64\r\n\r\n")
			buf.Flush()
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	noopSender := func(*string, *string) error { return nil }

	t.Run("stepfun stream error body read failure", func(t *testing.T) {
		srv := truncatedError(t)
		m := NewStepFunModel(map[string]string{"default": srv.URL}, URLSuffix{TTS: "tts"})
		err := m.AudioSpeechWithSender(t.Context(), ptr("voice"), ptr("hello"), apiConfig(), nil, nil, noopSender)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
		}
	})

	t.Run("togetherai stream error body read failure", func(t *testing.T) {
		srv := truncatedError(t)
		m := NewTogetherAIModel(map[string]string{"default": srv.URL}, URLSuffix{TTS: "tts"})
		err := m.AudioSpeechWithSender(t.Context(), ptr("voice"), ptr("hello"), apiConfig(), nil, nil, noopSender)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
		}
	})

	t.Run("fishaudio stream error body read failure", func(t *testing.T) {
		srv := truncatedError(t)
		m := NewFishAudioModel(map[string]string{"default": srv.URL}, URLSuffix{TTS: "tts"})
		err := m.AudioSpeechWithSender(t.Context(), ptr("voice"), ptr("hello"), apiConfig(), nil, nil, noopSender)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
		}
	})
}
