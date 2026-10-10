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

	"ragflow/internal/service/nav"
)

// fakeNavService records the routing request and returns canned nav hits. Only
// Search is exercised by navigate_tree; the rest satisfy the interface.
type fakeNavService struct {
	search func(ctx context.Context, tenantID, kbID, query string, embd []float32, docScope []string, topK int) ([]nav.NavHit, error)
}

func (f fakeNavService) UpsertDoc(context.Context, nav.UpsertDocInput) error { return nil }
func (f fakeNavService) RemoveDoc(context.Context, string, string, string) error {
	return nil
}
func (f fakeNavService) Search(ctx context.Context, tenantID, kbID, query string, embd []float32, docScope []string, topK int) ([]nav.NavHit, error) {
	if f.search != nil {
		return f.search(ctx, tenantID, kbID, query, embd, docScope, topK)
	}
	return nil, nil
}
func (f fakeNavService) ListClusters(context.Context, string, string, string, int, int) ([]nav.NavNode, int64, error) {
	return nil, 0, nil
}
func (f fakeNavService) ListChildren(context.Context, string, string, string, string, int, int) ([]nav.NavNode, int64, error) {
	return nil, 0, nil
}
func (f fakeNavService) SummariesByDocIDs(context.Context, string, string, []string) map[string]string {
	return nil
}

func installFakeNavService(t *testing.T, svc nav.NavService) {
	t.Helper()
	prev := nav.GetNavService()
	nav.SetNavService(svc)
	t.Cleanup(func() { nav.SetNavService(prev) })
}

// TestNavigateTreeToolIsRegisteredAndBuildableByTemplate pins the wiring a
// template needs: the name must be in the registry, and toolsFor must turn it
// into the navigate tool with the session scope injected.
func TestNavigateTreeToolIsRegisteredAndBuildableByTemplate(t *testing.T) {
	if _, ok := toolRegistry()[navigateTreeToolName]; !ok {
		t.Fatalf("%s must be registered so a template can select it", navigateTreeToolName)
	}
	tools := toolsFor(Template{ID: "nav", Tools: []string{navigateTreeToolName}}, "tenant-1", []string{"kb1"})
	if len(tools) != 1 {
		t.Fatalf("toolsFor returned %d tools, want 1", len(tools))
	}
	tool, ok := tools[0].(*NavigateTreeTool)
	if !ok {
		t.Fatalf("toolsFor built %T, want *NavigateTreeTool", tools[0])
	}
	info, err := tool.Info(context.Background())
	if err != nil || info.Name != navigateTreeToolName {
		t.Fatalf("tool info = %+v err=%v", info, err)
	}
	if tool.tenantID != "tenant-1" || len(tool.datasetIDs) != 1 || tool.datasetIDs[0] != "kb1" {
		t.Errorf("scope = %q/%v, want tenant-1/[kb1]", tool.tenantID, tool.datasetIDs)
	}
}

// TestNavigateTreeToolRoutesDocuments pins the payload contract: the routed
// documents with their summaries, ordered and capped.
func TestNavigateTreeToolRoutesDocuments(t *testing.T) {
	installFakeNavService(t, fakeNavService{search: func(_ context.Context, _ string, _ string, _ string, _ []float32, _ []string, _ int) ([]nav.NavHit, error) {
		return []nav.NavHit{
			{Type: nav.TypeNavDoc, DocID: "d1", Name: "Annual Report"},
			{Type: nav.TypeNavDoc, DocID: "d2", Name: "Quarterly Summary"},
		}, nil
	}})

	tool := NewNavigateTreeTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"2023 revenue by segment"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	for _, want := range []string{
		`<tree_navigation count="2" query="2023 revenue by segment">`,
		`<doc rank="1" doc_id="d1">`,
		`<summary>Annual Report</summary>`,
		`<doc rank="2" doc_id="d2">`,
		`<summary>Quarterly Summary</summary>`,
		`</tree_navigation>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output must contain %q, got:\n%s", want, out)
		}
	}
}

// TestNavigateTreeToolSkipsClusters: only nav_doc leaves route; a nav_cluster
// row must never become a routed "document".
func TestNavigateTreeToolSkipsClusters(t *testing.T) {
	installFakeNavService(t, fakeNavService{search: func(_ context.Context, _ string, _ string, _ string, _ []float32, _ []string, _ int) ([]nav.NavHit, error) {
		return []nav.NavHit{
			{Type: nav.TypeNavCluster, DocID: "kb1", Name: "Finance"},
			{Type: nav.TypeNavDoc, DocID: "d1", Name: "Annual Report"},
		}, nil
	}})

	tool := NewNavigateTreeTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"finance"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `count="1"`) || strings.Contains(out, `doc_id="kb1"`) {
		t.Errorf("clusters must not route, got:\n%s", out)
	}
}

// TestNavigateTreeToolNoTreeIsAHintNotAnError: a dataset without a compiled
// navigation tree (no NavService installed at all) is a dataset-level dead end
// the model reads and moves on from - not a Go error that would abort the ReAct
// loop.
func TestNavigateTreeToolNoTreeIsAHintNotAnError(t *testing.T) {
	installFakeNavService(t, nil) // no NavService installed -> Route returns (nil,nil)

	tool := NewNavigateTreeTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `count="0"`) || !strings.Contains(out, `error="no compiled navigation tree"`) {
		t.Errorf("no-tree must explain the absence, got:\n%s", out)
	}
}

// TestNavigateTreeToolNoDocOnEmptyRoute: a structure that exists but routes to
// nothing is a query-level miss carrying no error attribute.
func TestNavigateTreeToolNoDocOnEmptyRoute(t *testing.T) {
	installFakeNavService(t, fakeNavService{search: func(_ context.Context, _ string, _ string, _ string, _ []float32, _ []string, _ int) ([]nav.NavHit, error) {
		return []nav.NavHit{}, nil // structure exists, nothing routed
	}})

	tool := NewNavigateTreeTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `count="0"`) {
		t.Fatalf("empty route must report count=0, got:\n%s", out)
	}
	if strings.Contains(out, `error="`) {
		t.Errorf("a query-level miss must carry no error attribute, got:\n%s", out)
	}
}

// TestNavigateTreeToolRequiresQuery pins the argument contract.
func TestNavigateTreeToolRequiresQuery(t *testing.T) {
	tool := NewNavigateTreeTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"   "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("missing query must produce a <tool_error>, got:\n%s", out)
	}
}
