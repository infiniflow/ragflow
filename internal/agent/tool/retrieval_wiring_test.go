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

package tool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRetrievalTool_StubReturnsServiceMissing: with the default
// stub service installed, the tool still surfaces the no-service
// error so callers can distinguish "no real impl" from "real
// impl returned 0 chunks".
func TestRetrievalTool_StubReturnsServiceMissing(t *testing.T) {
	// Ensure stub is the active service.
	SetRetrievalService(nil)
	tool := NewRetrievalTool()
	_, err := tool.InvokableRun(context.Background(), `{"query":"hi"}`)
	if !errors.Is(err, ErrRetrievalServiceMissing) {
		t.Errorf("err=%v, want ErrRetrievalServiceMissing", err)
	}
}

// TestRetrievalTool_SimpleServiceReturnsChunks: with the simple
// test service installed, the tool returns synthetic chunks.
func TestRetrievalTool_SimpleServiceReturnsChunks(t *testing.T) {
	prev := GetRetrievalService()
	t.Cleanup(func() { SetRetrievalService(prev) })
	SetSimpleRetrievalService()

	tool := NewRetrievalTool()
	out, err := tool.InvokableRun(context.Background(),
		`{"query":"hello world","top_n":2}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "[ID:simple-0]") {
		t.Errorf("expected chunk with id simple-0 in output; got %s", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected query in chunk content; got %s", out)
	}
	if !strings.Contains(out, "[ID:simple-1]") {
		t.Errorf("expected chunk with id simple-1 in output; got %s", out)
	}
}

// TestSimpleRetrievalService_EmptyQuery: empty query → no chunks,
// no error.
func TestSimpleRetrievalService_EmptyQuery(t *testing.T) {
	chunks, err := simpleRetrievalService{}.Search(context.Background(), RetrievalRequest{Query: ""})
	if err != nil {
		t.Errorf("err: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty query, got %d", len(chunks))
	}
}

// TestSimpleRetrievalService_RespectsTopN: top_n=1 → exactly 1
// chunk.
func TestSimpleRetrievalService_RespectsTopN(t *testing.T) {
	chunks, err := simpleRetrievalService{}.Search(context.Background(), RetrievalRequest{
		Query: "x",
		TopN:  1,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for top_n=1, got %d", len(chunks))
	}
	if chunks[0].ID != "simple-0" {
		t.Errorf("expected simple-0, got %q", chunks[0].ID)
	}
}
