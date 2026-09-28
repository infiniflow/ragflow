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

package retrievalbridge

import (
	"context"
	"errors"
	"reflect"
	"testing"

	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
)

type captureMessageSearcher struct {
	userID string
	params map[string]any
}

func (s *captureMessageSearcher) SearchMessage(
	_ context.Context,
	userID string,
	_ map[string]any,
	params map[string]any,
) ([]map[string]any, common.ErrorCode, error) {
	s.userID = userID
	s.params = params
	return nil, common.CodeSuccess, nil
}

type fakeQueryTranslator struct {
	calls     int
	tenantID  string
	query     string
	languages []string
	result    string
	err       error
}

func (f *fakeQueryTranslator) CrossLanguages(
	_ context.Context,
	tenantID, query string,
	languages []string,
) (string, error) {
	f.calls++
	f.tenantID = tenantID
	f.query = query
	f.languages = languages
	return f.result, f.err
}

func memoryRequest(languages ...string) agenttool.RetrievalRequest {
	return agenttool.RetrievalRequest{
		Query:          "¿qué pasó ayer?",
		MemoryIDs:      []string{"mem-1"},
		TenantID:       "tenant-1",
		CrossLanguages: languages,
	}
}

func TestMemoryAdapterTranslatesQueryWithCrossLanguages(t *testing.T) {
	searcher := &captureMessageSearcher{}
	translator := &fakeQueryTranslator{result: "what happened yesterday?"}
	adapter := &MemoryAdapter{svc: searcher, translator: translator}

	if _, err := adapter.Search(t.Context(), nil, memoryRequest("English")); err != nil {
		t.Fatal(err)
	}
	if translator.calls != 1 {
		t.Fatalf("CrossLanguages calls = %d, want 1", translator.calls)
	}
	if translator.tenantID != "tenant-1" || translator.query != "¿qué pasó ayer?" ||
		!reflect.DeepEqual(translator.languages, []string{"English"}) {
		t.Fatalf("CrossLanguages args = (%q, %q, %#v)", translator.tenantID, translator.query, translator.languages)
	}
	if got := searcher.params["query"]; got != "what happened yesterday?" {
		t.Fatalf("searched query = %#v, want the translated query", got)
	}
}

func TestMemoryAdapterSkipsTranslationWithoutCrossLanguages(t *testing.T) {
	searcher := &captureMessageSearcher{}
	translator := &fakeQueryTranslator{result: "unused"}
	adapter := &MemoryAdapter{svc: searcher, translator: translator}

	if _, err := adapter.Search(t.Context(), nil, memoryRequest()); err != nil {
		t.Fatal(err)
	}
	if translator.calls != 0 {
		t.Fatalf("CrossLanguages calls = %d, want 0", translator.calls)
	}
	if got := searcher.params["query"]; got != "¿qué pasó ayer?" {
		t.Fatalf("searched query = %#v, want the original query", got)
	}
}

func TestMemoryAdapterKeepsOriginalQueryWhenTranslationFails(t *testing.T) {
	for _, tc := range []struct {
		name       string
		translator *fakeQueryTranslator
	}{
		{name: "error", translator: &fakeQueryTranslator{err: errors.New("no default chat model")}},
		{name: "blank result", translator: &fakeQueryTranslator{result: "  "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			searcher := &captureMessageSearcher{}
			adapter := &MemoryAdapter{svc: searcher, translator: tc.translator}

			if _, err := adapter.Search(t.Context(), nil, memoryRequest("English")); err != nil {
				t.Fatal(err)
			}
			if got := searcher.params["query"]; got != "¿qué pasó ayer?" {
				t.Fatalf("searched query = %#v, want the original query", got)
			}
		})
	}
}

func TestNewMemoryAdapterWithoutServiceReportsMissingService(t *testing.T) {
	adapter := NewMemoryAdapter(nil, &fakeQueryTranslator{})

	_, err := adapter.Search(t.Context(), nil, memoryRequest("English"))
	if !errors.Is(err, agenttool.ErrMemoryRetrievalServiceMissing) {
		t.Fatalf("err = %v, want ErrMemoryRetrievalServiceMissing", err)
	}
}

func TestMemoryAdapterRejectsCrossLanguagesWithoutTranslator(t *testing.T) {
	searcher := &captureMessageSearcher{}
	adapter := &MemoryAdapter{svc: searcher}

	if _, err := adapter.Search(t.Context(), nil, memoryRequest("English")); err == nil {
		t.Fatal("expected an error when cross_languages is set and no translator is configured")
	}
	if searcher.params != nil {
		t.Fatal("memory search must not run with an untranslated query")
	}
}
