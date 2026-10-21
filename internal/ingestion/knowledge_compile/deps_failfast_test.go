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

package knowledge_compile

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// The doc engine is a hard dependency of the dataset-level compile reader,
// writer, and contribution store. Before dependency injection, a nil engine
// silently degraded: readers reported "no products", writers reported
// "deleted successfully", and the contribution store silently fell back to an
// in-memory map — a batch would be acked as merged while nothing was read,
// written, or deleted. These tests pin the fail-fast contract: an unconfigured
// engine is a loud error, never a silent success.

const engineUnconfiguredNeedle = "engine"

func requireEngineError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a fail-fast error for the unconfigured doc engine, got nil (silent success)")
	}
	if !strings.Contains(err.Error(), engineUnconfiguredNeedle) {
		t.Fatalf("error %q does not identify the missing doc engine", err.Error())
	}
}

func TestEngineReaderWithoutEngineFailsFast(t *testing.T) {
	ctx := context.Background()
	r := engineReader{} // zero value: no engine injected

	cases := []struct {
		name string
		call func() error
	}{
		{"LoadDocProducts", func() error {
			_, err := r.LoadDocProducts(ctx, "tenant-1", "kb-1", "doc-1")
			return err
		}},
		{"LoadMergedProduct", func() error {
			_, err := r.LoadMergedProduct(ctx, "tenant-1", "kb-1", "product-1")
			return err
		}},
		{"LoadMergedWikiPages", func() error {
			_, err := r.LoadMergedWikiPages(ctx, "tenant-1", "kb-1")
			return err
		}},
		{"LoadDocumentWikiPagesBySlugs", func() error {
			_, err := r.LoadDocumentWikiPagesBySlugs(ctx, "tenant-1", "kb-1", []string{"slug-1"})
			return err
		}},
		{"LoadDocumentWikiPagesBySlugs with empty slugs", func() error {
			_, err := r.LoadDocumentWikiPagesBySlugs(ctx, "tenant-1", "kb-1", nil)
			return err
		}},
		{"SearchSimilar", func() error {
			_, _, err := r.SearchSimilar(ctx, "tenant-1", "kb-1", kccommon.VariantWiki, []float64{0.1, 0.2}, 5, 0.85)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireEngineError(t, tc.call())
		})
	}
}

func TestEngineWriterWithoutEngineFailsFast(t *testing.T) {
	ctx := context.Background()
	// Zero value: no engine injected. commitService and resolveLanguage stay
	// nil so the engine check is the only failure mode under test.
	w := engineWriter{}

	oneDoc := []string{"doc-1"}
	oneProduct := []kccommon.Product{{ID: "product-1"}}

	cases := []struct {
		name string
		call func() error
	}{
		{"DeleteMergedWikiPages", func() error {
			return w.DeleteMergedWikiPages(ctx, "tenant-1", "kb-1", []string{"page-1"})
		}},
		{"WriteMerged", func() error {
			return w.WriteMerged(ctx, "tenant-1", "kb-1", oneProduct)
		}},
		{"WriteMergedStructure", func() error {
			return w.WriteMergedStructure(ctx, "tenant-1", "kb-1", []StructureBucket{{}})
		}},
		{"DeleteStructureForDocs", func() error {
			return w.DeleteStructureForDocs(ctx, "tenant-1", "kb-1", oneDoc)
		}},
		{"DeleteDocLevelForDocs", func() error {
			return w.DeleteDocLevelForDocs(ctx, "tenant-1", "kb-1", oneDoc)
		}},
		{"StripMergedSources", func() error {
			return w.StripMergedSources(ctx, "tenant-1", "kb-1", oneDoc)
		}},
		{"ProjectWikiGraph", func() error {
			return w.ProjectWikiGraph(ctx, "tenant-1", "kb-1")
		}},
		{"DropWikiGraph", func() error {
			return w.DropWikiGraph(ctx, "tenant-1", "kb-1")
		}},
		{"DeleteMerged", func() error {
			return w.DeleteMerged(ctx, "tenant-1", "kb-1")
		}},
		{"DeleteMergedForVariant", func() error {
			return w.DeleteMergedForVariant(ctx, "tenant-1", "kb-1", []kccommon.Variant{kccommon.VariantWiki})
		}},
		{"loadWikiPageContent", func() error {
			_, err := w.loadWikiPageContent(ctx, "tenant-1", "kb-1", oneProduct)
			return err
		}},
		{"loadActiveDocumentWikiPages", func() error {
			_, err := w.loadActiveDocumentWikiPages(ctx, "tenant-1", "kb-1")
			return err
		}},
		{"loadMergedWikiPages", func() error {
			_, err := w.loadMergedWikiPages(ctx, "tenant-1", "kb-1")
			return err
		}},
		{"dropWikiGraph", func() error {
			return w.dropWikiGraph(ctx, "tenant-1", "kb-1")
		}},
		{"insertWikiGraphChunks", func() error {
			return w.insertWikiGraphChunks(ctx, "tenant-1", "kb-1", []map[string]interface{}{{"id": "row-1"}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireEngineError(t, tc.call())
		})
	}
}

func TestNewConsumerWithoutConfiguredStorageFailsFast(t *testing.T) {
	// A Consumer assembled without an injected doc engine must fail loudly on
	// the first storage access. Previously it silently read zero products and
	// acked the batch as merged.
	consumer := NewConsumer(NewFakeScheduler())
	if consumer.IsStorageConfigured() {
		t.Fatal("unconfigured consumer must report IsStorageConfigured() == false")
	}
	err := consumer.processBatch(context.Background(), "tenant-1", "kb-1", "claim-token-1",
		[]BacklogEntry{{DocID: "doc-1", EventType: string(EventTypeCompleted), Variants: []string{"wiki"}}},
		[]string{kccommon.TaskTypeWiki})
	requireEngineError(t, err)
}

// TestWithStorageConfiguresReaderWriter pins the contract that WithStorage
// (the production wiring path) installs an engine-backed reader, writer, and
// contribution store. Without this, a bare NewConsumer silently wires nothing
// and the production merge path is dead.
func TestWithStorageConfiguresReaderWriter(t *testing.T) {
	consumer := NewConsumer(NewFakeScheduler(), WithStorage(&fakeEngine{engineType: "infinity"}, nil))
	if !consumer.IsStorageConfigured() {
		t.Fatal("WithStorage must report IsStorageConfigured() == true")
	}
}

// TestWithStorageInjectsDatabaseHandle pins the contract that WithStorage wires
// the database handle into the consumer, so the storage-backed paths (doc
// enumeration, dataset compile log) read from the injected db rather than a
// package-level global installed by Provision. This decouples the consumer from
// Provision's call ordering and from the knowledge_compile-specific global kcDB.
func TestWithStorageInjectsDatabaseHandle(t *testing.T) {
	db := &gorm.DB{}
	consumer := NewConsumer(NewFakeScheduler(), WithStorage(&fakeEngine{engineType: "infinity"}, db))
	if consumer.db != db {
		t.Fatalf("WithStorage did not inject the database handle: got %v, want %v", consumer.db, db)
	}
}

func TestNewWikiContributionStoreWithoutEngineFailsFast(t *testing.T) {
	// Previously a nil engine silently produced an in-memory store, so a
	// mis-wired consumer diffed contributions against empty state.
	ctx := context.Background()
	store := newWikiContributionStore(nil)

	cases := []struct {
		name string
		call func() error
	}{
		{"Get", func() error {
			_, _, err := store.Get(ctx, "tenant-1", "kb-1", "doc-1")
			return err
		}},
		{"Put", func() error {
			return store.Put(ctx, "tenant-1", "kb-1", wikiDocumentContribution{DocumentID: "doc-1"})
		}},
		{"Delete", func() error {
			return store.Delete(ctx, "tenant-1", "kb-1", "doc-1")
		}},
		{"ClearDataset", func() error {
			return store.ClearDataset(ctx, "tenant-1", "kb-1")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireEngineError(t, tc.call())
		})
	}
}
