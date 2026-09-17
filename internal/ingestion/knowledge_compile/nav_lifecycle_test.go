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

package knowledge_compile

import (
	"context"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/service/nav"
)

type recordingNavService struct {
	removed   []string
	upserted  []nav.UpsertDocInput
	removeErr error
}

func (f *recordingNavService) UpsertDoc(_ context.Context, input nav.UpsertDocInput) error {
	f.upserted = append(f.upserted, input)
	return nil
}

func (f *recordingNavService) RemoveDoc(_ context.Context, _, _, docID string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, docID)
	return nil
}

func (f *recordingNavService) Search(context.Context, string, string, string, []float32, []string, int) ([]nav.NavHit, error) {
	return nil, nil
}

func (f *recordingNavService) ListClusters(context.Context, string, string, string, int, int) ([]nav.NavNode, int64, error) {
	return nil, 0, nil
}

func (f *recordingNavService) ListChildren(context.Context, string, string, string, string, int, int) ([]nav.NavNode, int64, error) {
	return nil, 0, nil
}

func (f *recordingNavService) SummariesByDocIDs(context.Context, string, string, []string) map[string]string {
	return nil
}

func TestProcessBatchRemovesRetractedDocumentsFromNavigation(t *testing.T) {
	previous := nav.GetNavService()
	recorder := &recordingNavService{}
	nav.SetNavService(recorder)
	defer nav.SetNavService(previous)

	consumer := NewConsumer(
		NewFakeScheduler(),
		WithReader(&fakeReader{}),
		WithWriter(&fakeWriter{}),
		withWikiContributionStore(&memoryWikiContributionStore{items: map[string]wikiDocumentContribution{}}),
	)

	err := consumer.processBatch(context.Background(), "tenant-1", "kb-1", "", []BacklogEntry{
		{DocID: "doc-disabled", EventType: string(EventTypeDisabled)},
		{DocID: "doc-deleted", EventType: string(EventTypeDeleted)},
	}, nil)
	if err != nil {
		t.Fatalf("processBatch failed: %v", err)
	}
	if len(recorder.removed) != 2 {
		t.Fatalf("removed navigation docs = %v, want two documents", recorder.removed)
	}
	if recorder.removed[0] != "doc-deleted" || recorder.removed[1] != "doc-disabled" {
		t.Fatalf("removed navigation docs = %v, want sorted document ids", recorder.removed)
	}
}

func TestProcessBatchRetriesWhenNavigationRemovalFails(t *testing.T) {
	previous := nav.GetNavService()
	recorder := &recordingNavService{removeErr: context.Canceled}
	nav.SetNavService(recorder)
	defer nav.SetNavService(previous)

	consumer := NewConsumer(
		NewFakeScheduler(),
		WithReader(&fakeReader{}),
		WithWriter(&fakeWriter{}),
		withWikiContributionStore(&memoryWikiContributionStore{items: map[string]wikiDocumentContribution{}}),
	)

	err := consumer.processBatch(context.Background(), "tenant-1", "kb-1", "", []BacklogEntry{
		{DocID: "doc-disabled", EventType: string(EventTypeDisabled)},
	}, nil)
	if err == nil {
		t.Fatal("processBatch should fail when navigation removal fails so the batch can be retried")
	}
}

func TestProcessBatchReaddsEnabledDocumentToNavigation(t *testing.T) {
	previous := nav.GetNavService()
	recorder := &recordingNavService{}
	nav.SetNavService(recorder)
	defer nav.SetNavService(previous)

	consumer := NewConsumer(
		NewFakeScheduler(),
		WithReader(&fakeReader{products: []kccommon.Product{{
			DocID:    "doc-enabled",
			TenantID: "tenant-1",
			Variant:  kccommon.VariantTree,
			Content:  "enabled document summary",
			Meta:     map[string]any{"kind": "root"},
		}}}),
		WithWriter(&fakeWriter{}),
		WithDeduperFactory(func(string) (Deduper, error) { return NewNoopDeduper(), nil }),
		withWikiContributionStore(&memoryWikiContributionStore{items: map[string]wikiDocumentContribution{}}),
	)

	err := consumer.processBatch(context.Background(), "tenant-1", "kb-1", "", []BacklogEntry{
		{DocID: "doc-enabled", EventType: string(EventTypeEnabled), Variants: []string{string(kccommon.VariantTree)}},
	}, nil)
	if err != nil {
		t.Fatalf("processBatch failed: %v", err)
	}
	if len(recorder.upserted) != 1 {
		t.Fatalf("upserted navigation docs = %d, want 1", len(recorder.upserted))
	}
	if recorder.upserted[0].DocID != "doc-enabled" || recorder.upserted[0].Summary != "enabled document summary" {
		t.Fatalf("upserted navigation input = %+v, want enabled document summary", recorder.upserted[0])
	}
}
