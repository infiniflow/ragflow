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

package agentic_rag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/common"
)

// fakeMetadataService is a metadataResolver implementation with no external
// dependencies, driving the search_metadata tool through every branch.
type fakeMetadataService struct {
	fields     []common.MetadataFieldDef
	metas      common.MetaData
	pushdown   []string
	pushdownOK bool
	docMeta    map[string]map[string]any
}

func (f *fakeMetadataService) FilterDocIDsByMetaPushdown(_ context.Context, _ []string, _ []map[string]any, _ string) ([]string, bool) {
	return f.pushdown, f.pushdownOK
}

func (f *fakeMetadataService) GetFlattedMetaByKBs(_ context.Context, _ []string) (common.MetaData, error) {
	return f.metas, nil
}

func (f *fakeMetadataService) MetadataForDocIDs(_ context.Context, _ []string, _ []string) (map[string]map[string]any, error) {
	return f.docMeta, nil
}

func (f *fakeMetadataService) DeclaredMetadataFields(_ context.Context, _ []string) ([]common.MetadataFieldDef, error) {
	return f.fields, nil
}

type msResult struct {
	Kind      string   `json:"kind"`
	DocIDs    []string `json:"doc_ids"`
	Documents []struct {
		DocID    string         `json:"doc_id"`
		Metadata map[string]any `json:"metadata"`
	} `json:"documents"`
	Note string `json:"note"`
}

// runMetadataSearch wires the given resolver and runs the tool, failing the test
// on a transport-level error (the tool converts its own failures to a model
// result, so InvokableRun should only error on malformed input).
func runMetadataSearch(t *testing.T, svc metadataResolver, args string) msResult {
	t.Helper()
	SetMetadataService(svc)
	defer SetMetadataService(nil)
	out, err := NewMetadataSearchTool("t", []string{"kb1"}).InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	var r msResult
	if e := json.Unmarshal([]byte(out), &r); e != nil {
		t.Fatalf("output is not JSON: %q: %v", out, e)
	}
	return r
}

func TestMetadataSearchTool_InfoParsesSchema(t *testing.T) {
	if _, err := NewMetadataSearchTool("t", nil).Info(context.Background()); err != nil {
		t.Fatalf("Info schema failed to parse: %v", err)
	}
}

func TestMetadataSearchTool_NoFilters(t *testing.T) {
	r := runMetadataSearch(t, &fakeMetadataService{}, `{"filters":[]}`)
	if r.Note == "" || !strings.Contains(r.Note, "No metadata conditions given") {
		t.Fatalf("want 'No metadata conditions given' note, got %q", r.Note)
	}
	if len(r.DocIDs) != 0 {
		t.Fatalf("expected empty doc_ids, got %v", r.DocIDs)
	}
}

func TestMetadataSearchTool_NoResolver(t *testing.T) {
	r := runMetadataSearch(t, nil, `{"filters":[{"key":"author","op":"contains","value":"X"}]}`)
	if !strings.Contains(r.Note, "unavailable") {
		t.Fatalf("want 'unavailable' note, got %q", r.Note)
	}
}

func TestMetadataSearchTool_BadKey(t *testing.T) {
	r := runMetadataSearch(t, &fakeMetadataService{}, `{"filters":[{"key":"nope","op":"contains","value":"X"}]}`)
	if !strings.Contains(r.Note, "do not exist") {
		t.Fatalf("want 'do not exist' note, got %q", r.Note)
	}
}

func TestMetadataSearchTool_MatchViaPushdown(t *testing.T) {
	svc := &fakeMetadataService{
		fields:     []common.MetadataFieldDef{{Key: "author", Type: "string"}},
		pushdown:   []string{"doc1"},
		pushdownOK: true,
	}
	r := runMetadataSearch(t, svc, `{"filters":[{"key":"author","op":"contains","value":"Zhang"}]}`)
	if len(r.DocIDs) != 1 || r.DocIDs[0] != "doc1" {
		t.Fatalf("want [doc1], got %v", r.DocIDs)
	}
}

func TestMetadataSearchTool_FallbackMetaFilter(t *testing.T) {
	svc := &fakeMetadataService{
		fields: []common.MetadataFieldDef{{Key: "author", Type: "string"}},
		metas:  common.MetaData{"author": {"Zhang San": []string{"doc1"}}},
	}
	r := runMetadataSearch(t, svc, `{"filters":[{"key":"author","op":"contains","value":"Zhang"}]}`)
	if len(r.DocIDs) != 1 || r.DocIDs[0] != "doc1" {
		t.Fatalf("fallback failed, want [doc1], got %v", r.DocIDs)
	}
}

func TestMetadataSearchTool_NoMatch(t *testing.T) {
	svc := &fakeMetadataService{
		fields:     []common.MetadataFieldDef{{Key: "author", Type: "string"}},
		pushdown:   []string{},
		pushdownOK: true,
	}
	r := runMetadataSearch(t, svc, `{"filters":[{"key":"author","op":"contains","value":"Nobody"}]}`)
	if !strings.Contains(r.Note, "No documents match") {
		t.Fatalf("want 'No documents match' note, got %q", r.Note)
	}
}

func TestMetadataSearchTool_ContextDocs(t *testing.T) {
	svc := &fakeMetadataService{
		fields:     []common.MetadataFieldDef{{Key: "author", Type: "string"}},
		pushdown:   []string{"doc1"},
		pushdownOK: true,
		docMeta:    map[string]map[string]any{"doc1": {"author": "Zhang San", "year": "2024"}},
	}
	r := runMetadataSearch(t, svc, `{"filters":[{"key":"author","op":"contains","value":"Zhang"}]}`)
	if len(r.Documents) != 1 || r.Documents[0].DocID != "doc1" {
		t.Fatalf("want one document doc1, got %+v", r.Documents)
	}
	if got, ok := r.Documents[0].Metadata["author"]; !ok || got != "Zhang San" {
		t.Fatalf("want author=Zhang San in metadata, got %+v", r.Documents[0].Metadata)
	}
}
