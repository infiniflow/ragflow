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

package agentic_rag

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestEvidenceRegistryStampsInFirstSeenOrder(t *testing.T) {
	r := NewEvidenceRegistry()
	if got := r.Stamp("c1"); got != 0 {
		t.Fatalf("Stamp(c1) = %d, want 0", got)
	}
	if got := r.Stamp("c2"); got != 1 {
		t.Fatalf("Stamp(c2) = %d, want 1", got)
	}
	// A passage that reaches the model twice keeps its first number.
	if got := r.Stamp("c1"); got != 0 {
		t.Fatalf("Stamp(c1) again = %d, want the stable 0", got)
	}
	if got := r.Stamp("   "); got != -1 {
		t.Fatalf("Stamp(blank) = %d, want -1 (not citable)", got)
	}

	ids := r.IDs()
	if len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Fatalf("IDs() = %v, want [c1 c2]", ids)
	}
	// The snapshot must be a copy: a caller cannot mutate the registry through it.
	ids[0] = "mutated"
	if r.IDs()[0] != "c1" {
		t.Error("IDs() returned a live slice, not a copy")
	}
}

func TestEvidenceRegistryResolve(t *testing.T) {
	r := NewEvidenceRegistry()
	r.Stamp("c1")
	if id, ok := r.Resolve(0); !ok || id != "c1" {
		t.Fatalf("Resolve(0) = (%q, %v), want (c1, true)", id, ok)
	}
	if _, ok := r.Resolve(1); ok {
		t.Error("Resolve(1) resolved an unpublished handle")
	}
	if _, ok := r.Resolve(-1); ok {
		t.Error("Resolve(-1) resolved")
	}
}

func TestEvidenceRegistryNilSafety(t *testing.T) {
	var r *EvidenceRegistry
	if got := r.Stamp("x"); got != -1 {
		t.Errorf("nil.Stamp = %d, want -1", got)
	}
	if r.IDs() != nil {
		t.Error("nil.IDs() must be nil")
	}
	if _, ok := r.Resolve(0); ok {
		t.Error("nil.Resolve resolved")
	}
}

func TestEvidenceRegistryRidesOnContext(t *testing.T) {
	r := NewEvidenceRegistry()
	ctx := WithEvidenceRegistry(context.Background(), r)
	if got := evidenceRegistryFrom(ctx); got != r {
		t.Fatal("the registry did not travel on the context")
	}
	if evidenceRegistryFrom(context.Background()) != nil {
		t.Error("a bare context must carry no registry")
	}
	// A nil registry is a no-op, not a nil-valued key.
	if WithEvidenceRegistry(context.Background(), nil) != context.Background() {
		t.Error("WithEvidenceRegistry(nil) must return the same context")
	}
}

// The locate payload stamps each served passage with its [ID:n] handle when the
// run carries a registry, and stamps nothing when it does not.
func TestLocateResultsStampRefHandle(t *testing.T) {
	hits := []snippetHit{
		{chunk: runtime.RetrievalChunk{ID: "c1"}},
		{chunk: runtime.RetrievalChunk{ID: "c2"}},
	}
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	out := formatLocateResultsXML(ctx, "grep_chunks", "q", hits)
	if !strings.Contains(out, `ref="0"`) || !strings.Contains(out, `ref="1"`) {
		t.Fatalf("served passages not stamped with their handles: %.300q", out)
	}

	plain := formatLocateResultsXML(context.Background(), "grep_chunks", "q", hits)
	if strings.Contains(plain, `ref="`) {
		t.Errorf("a run with no registry must publish no handle: %.300q", plain)
	}
}

// Deep-read passages are citable too.
func TestChunksXMLStampsRefHandle(t *testing.T) {
	chunks := []runtime.RetrievalChunk{{ID: "d1"}, {ID: "d2"}}
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	out := formatChunksXML(ctx, "doc-1", chunks, "", "")
	if !strings.Contains(out, `ref="0"`) || !strings.Contains(out, `ref="1"`) {
		t.Fatalf("deep-read passages not stamped: %.300q", out)
	}
}
