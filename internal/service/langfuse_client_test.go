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

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// langfuseIngestionStub captures the batch payload the client posts to
// /api/public/ingestion so tests can assert the v3 envelope shape.
type langfuseIngestionStub struct {
	requests chan []byte
	server   *httptest.Server
}

func newLangfuseIngestionStub(t *testing.T) *langfuseIngestionStub {
	t.Helper()
	stub := &langfuseIngestionStub{requests: make(chan []byte, 16)}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/ingestion" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
		body, _ := io.ReadAll(r.Body)
		stub.requests <- body
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

type langfuseBatchEnvelope struct {
	Batch []struct {
		ID        string          `json:"id"`
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp"`
		Body      json.RawMessage `json:"body"`
	} `json:"batch"`
}

func waitForLangfuseBatch(t *testing.T, stub *langfuseIngestionStub) langfuseBatchEnvelope {
	t.Helper()
	select {
	case raw := <-stub.requests:
		var env langfuseBatchEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("unmarshal ingestion payload: %v", err)
		}
		if len(env.Batch) != 1 {
			t.Fatalf("batch size = %d, want 1", len(env.Batch))
		}
		return env
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for ingestion request")
		return langfuseBatchEnvelope{}
	}
}

func TestLangfuseClient_PostTrace_BatchIngestion(t *testing.T) {
	stub := newLangfuseIngestionStub(t)
	c := NewLangfuseClient(stub.server.URL, "pk", "sk")
	defer c.Shutdown(context.Background())

	if err := c.PostTrace(context.Background(), LangfuseTrace{ID: "trace-1", Name: "chat"}); err != nil {
		t.Fatalf("PostTrace: %v", err)
	}
	env := waitForLangfuseBatch(t, stub)
	if env.Batch[0].Type != "trace-create" {
		t.Errorf("batch type = %q, want trace-create", env.Batch[0].Type)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(env.Batch[0].Body, &body); err != nil {
		t.Fatalf("unmarshal event body: %v", err)
	}
	if body["id"] != "trace-1" {
		t.Errorf("body.id = %v, want trace-1", body["id"])
	}
}

func TestLangfuseClient_PostGeneration_CreateVsUpdate(t *testing.T) {
	stub := newLangfuseIngestionStub(t)
	c := NewLangfuseClient(stub.server.URL, "pk", "sk")
	defer c.Shutdown(context.Background())

	if err := c.PostGeneration(context.Background(), LangfuseGeneration{ID: "gen-1", TraceID: "trace-1", Model: "m"}); err != nil {
		t.Fatalf("PostGeneration start: %v", err)
	}
	if env := waitForLangfuseBatch(t, stub); env.Batch[0].Type != "generation-create" {
		t.Errorf("start-side type = %q, want generation-create", env.Batch[0].Type)
	}

	if err := c.PostGeneration(context.Background(), LangfuseGeneration{ID: "gen-1", TraceID: "trace-1", Usage: &LangfuseUsage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}}); err != nil {
		t.Fatalf("PostGeneration end: %v", err)
	}
	env := waitForLangfuseBatch(t, stub)
	if env.Batch[0].Type != "generation-update" {
		t.Errorf("end-side type = %q, want generation-update", env.Batch[0].Type)
	}
	var body struct {
		Usage struct {
			Input  int `json:"input"`
			Output int `json:"output"`
			Total  int `json:"total"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(env.Batch[0].Body, &body); err != nil {
		t.Fatalf("unmarshal generation body: %v", err)
	}
	if body.Usage.Input != 3 || body.Usage.Output != 5 || body.Usage.Total != 8 {
		t.Errorf("usage = %+v, want input/output/total 3/5/8 (v3 schema tags)", body.Usage)
	}
}

func TestLangfuseClient_PostSpan_UpdateByEndTime(t *testing.T) {
	stub := newLangfuseIngestionStub(t)
	c := NewLangfuseClient(stub.server.URL, "pk", "sk")
	defer c.Shutdown(context.Background())

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := c.PostSpan(context.Background(), LangfuseSpan{ID: "sp-1", TraceID: "trace-1", StartTime: now}); err != nil {
		t.Fatalf("PostSpan start: %v", err)
	}
	if env := waitForLangfuseBatch(t, stub); env.Batch[0].Type != "span-create" {
		t.Errorf("start-side type = %q, want span-create", env.Batch[0].Type)
	}
	if err := c.PostSpan(context.Background(), LangfuseSpan{ID: "sp-1", TraceID: "trace-1", StartTime: now, EndTime: now}); err != nil {
		t.Fatalf("PostSpan end: %v", err)
	}
	if env := waitForLangfuseBatch(t, stub); env.Batch[0].Type != "span-update" {
		t.Errorf("end-side type = %q, want span-update", env.Batch[0].Type)
	}
}

func TestLangfuseClient_ShutdownDrainsPendingEvents(t *testing.T) {
	stub := newLangfuseIngestionStub(t)
	c := NewLangfuseClient(stub.server.URL, "pk", "sk")

	for i := 0; i < 3; i++ {
		if err := c.PostTrace(context.Background(), LangfuseTrace{ID: "trace-" + string(rune('a'+i))}); err != nil {
			t.Fatalf("PostTrace %d: %v", i, err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	deadline := time.After(3 * time.Second)
	got := 0
	for got < 3 {
		select {
		case <-stub.requests:
			got++
		case <-deadline:
			t.Fatalf("drained %d/3 events before deadline", got)
		}
	}
}
