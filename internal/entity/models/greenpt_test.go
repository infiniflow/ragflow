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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGreenPTListModelsClassifiesSpeech(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"glm-5.2"},{"id":"green-embedding"},{"id":"green-rerank"},{"id":"green-s"},{"id":"green-s-pro"}]}`)
	}))
	defer server.Close()

	driver := NewGreenPTModel(map[string]string{"default": server.URL}, URLSuffix{Models: "v1/models"})
	key := "test"
	models, err := driver.ListModels(context.Background(), &APIConfig{ApiKey: &key})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"glm-5.2":         "chat",
		"green-embedding": "embedding",
		"green-rerank":    "rerank",
		"green-s":         "asr",
		"green-s-pro":     "asr",
	}
	if len(models) != len(want) {
		t.Fatalf("expected %d models, got %d", len(want), len(models))
	}
	for _, model := range models {
		expectedType, ok := want[model.Name]
		if !ok {
			t.Fatalf("unexpected model: %s", model.Name)
		}
		if len(model.ModelTypes) != 1 || model.ModelTypes[0] != expectedType {
			t.Fatalf("%s classified as %v", model.Name, model.ModelTypes)
		}
		delete(want, model.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing expected models: %v", want)
	}
}

func TestGreenPTTranscribeAudio(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/listen" || r.URL.Query().Get("model") != "green-s" {
			t.Errorf("unexpected request URL: %s", r.URL.String())
			return
		}
		if r.Header.Get("Authorization") != "Token test-key" {
			t.Errorf("unexpected authorization header: %s", r.Header.Get("Authorization"))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}
		if string(body) != "audio" {
			t.Errorf("unexpected audio body: %q", body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":{"channels":[{"alternatives":[{"transcript":"renewable inference"}]}]}}`)
	}))
	defer server.Close()

	audio, err := os.CreateTemp(t.TempDir(), "*.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = audio.WriteString("audio"); err != nil {
		t.Fatal(err)
	}
	if err = audio.Close(); err != nil {
		t.Fatal(err)
	}

	driver := NewGreenPTModel(map[string]string{"default": server.URL}, URLSuffix{ASR: "v1/listen"})
	key, model, path := "test-key", "green-s", audio.Name()
	response, err := driver.TranscribeAudio(context.Background(), &model, &path, &APIConfig{ApiKey: &key}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "renewable inference" {
		t.Fatalf("unexpected transcript: %q", response.Text)
	}
}
