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
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestGraphDocOutlineToolRejectsDocumentOutsideBoundScope(t *testing.T) {
	fake := &fakeKgService{}
	installFakeKgService(t, fake)

	tool := NewGraphDocOutlineTool("tenant-1", []string{"kb1"})
	ctx := withDocumentScope(kgExploreContext(t), []string{"d1"})
	out, err := tool.InvokableRun(ctx, `{"query":"q","doc_ids":["d2"]}`)
	if err != nil {
		t.Fatalf("out-of-scope document must not abort the loop: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) || !strings.Contains(out, "outside the conversation") {
		t.Errorf("expected a <tool_error> naming the bound scope, got:\n%s", out)
	}
	if fake.gotOutline.DocIDs != nil {
		t.Errorf("the outline must not read for an out-of-scope document, got %+v", fake.gotOutline)
	}
}

func TestGraphDocOutlineToolKeepsInScopeDocIDs(t *testing.T) {
	fake := &fakeKgService{}
	installFakeKgService(t, fake)

	tool := NewGraphDocOutlineTool("tenant-1", []string{"kb1"})
	ctx := withDocumentScope(kgExploreContext(t), []string{"d1", "d2"})
	if _, err := tool.InvokableRun(ctx, `{"query":"q","doc_ids":["d2"]}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if len(fake.gotOutline.DocIDs) != 1 || fake.gotOutline.DocIDs[0] != "d2" {
		t.Errorf("outline doc ids = %v, want [d2]", fake.gotOutline.DocIDs)
	}
}

func TestGraphDocOutlineToolLocatesWithinBoundScope(t *testing.T) {
	fake := &fakeKgService{}
	installFakeKgService(t, fake)

	stub := &bm25StubService{}
	runtime.SetBm25Service(stub)
	defer runtime.SetBm25Service(nil)

	tool := NewGraphDocOutlineTool("tenant-1", []string{"kb1"})
	ctx := withDocumentScope(kgExploreContext(t), []string{"d1"})
	if _, err := tool.InvokableRun(ctx, `{"query":"q"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if len(stub.lastReq.DocScope) != 1 || stub.lastReq.DocScope[0] != "d1" {
		t.Errorf("locate doc scope = %v, want [d1]", stub.lastReq.DocScope)
	}
	if fake.gotOutline.DocIDs != nil {
		t.Errorf("the outline must not read documents the locate pass did not return, got %+v", fake.gotOutline)
	}
}
