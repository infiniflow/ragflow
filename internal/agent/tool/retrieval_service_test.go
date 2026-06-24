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
	"testing"
)

// TestRetrievalService_DefaultStub: with no SetRetrievalService
// call, the default stub returns ErrRetrievalServiceMissing.
func TestRetrievalService_DefaultStub(t *testing.T) {
	svc := GetRetrievalService()
	if svc == nil {
		t.Fatal("GetRetrievalService returned nil")
	}
	_, err := svc.Search(context.Background(), RetrievalRequest{Query: "hi"})
	if !errors.Is(err, ErrRetrievalServiceMissing) {
		t.Errorf("err=%v, want ErrRetrievalServiceMissing", err)
	}
}

// TestRetrievalService_RegisterAndRestore: a custom impl can be
// installed and later restored to the stub via SetRetrievalService(nil).
func TestRetrievalService_RegisterAndRestore(t *testing.T) {
	prev := GetRetrievalService()
	t.Cleanup(func() { SetRetrievalService(prev) })

	// Install a fake.
	SetRetrievalService(fakeRetrievalService{chunks: []RetrievalChunk{{ID: "1", Content: "fake"}}})
	got, err := GetRetrievalService().Search(context.Background(), RetrievalRequest{Query: "x"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].ID != "1" {
		t.Errorf("got %+v, want 1 chunk with id=1", got)
	}

	// Restore via nil.
	SetRetrievalService(nil)
	if _, err := GetRetrievalService().Search(context.Background(), RetrievalRequest{}); !errors.Is(err, ErrRetrievalServiceMissing) {
		t.Errorf("after nil-reset: err=%v, want ErrRetrievalServiceMissing", err)
	}
}

// fakeRetrievalService is a programmable stub for testing.
type fakeRetrievalService struct {
	chunks []RetrievalChunk
	err    error
}

func (f fakeRetrievalService) Search(_ context.Context, _ RetrievalRequest) ([]RetrievalChunk, error) {
	return f.chunks, f.err
}

// TestRetrievalChunk_Fields: round-trip the struct.
func TestRetrievalChunk_Fields(t *testing.T) {
	c := RetrievalChunk{ID: "i", Content: "c", DocumentID: "d", Score: 0.5}
	if c.ID != "i" || c.Content != "c" || c.DocumentID != "d" || c.Score != 0.5 {
		t.Errorf("round-trip lost fields: %+v", c)
	}
}
