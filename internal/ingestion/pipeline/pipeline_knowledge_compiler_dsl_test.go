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

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"ragflow/internal/dao"
	"testing"

	// Named import so this file can drive the runtime factory directly.
	"ragflow/internal/agent/runtime"
	// Blank-imports fire the runtime.MustRegister init hooks that populate
	// DefaultRegistry with File/Parser/TokenChunker/KnowledgeCompiler.
	_ "ragflow/internal/ingestion/component"
	_ "ragflow/internal/ingestion/component/chunker"
	_ "ragflow/internal/ingestion/component/knowledge_compiler"

	kc "ragflow/internal/ingestion/component/knowledge_compiler/common"

	"gorm.io/gorm"
)

// installStubResolvers wires in-memory GroupResolver / TemplateResolver stubs so
// the DSL tests can exercise the template-resolution path without MySQL.
func installStubResolvers(t *testing.T, groupToTemplates map[string][]string, templates map[string]kc.TemplateInfo) {
	t.Helper()
	kc.SetGroupResolver(func(ctx context.Context, db *gorm.DB, tenantID string, groupIDs []string) ([]string, error) {
		var out []string
		for _, g := range groupIDs {
			out = append(out, groupToTemplates[g]...)
		}
		return out, nil
	})
	kc.SetTemplateResolver(func(ctx context.Context, db *gorm.DB, tenantID, templateID string) (kc.TemplateInfo, error) {
		info, ok := templates[templateID]
		if !ok {
			return kc.TemplateInfo{}, fmt.Errorf("no stub template %q", templateID)
		}
		return info, nil
	})
	t.Cleanup(func() { kc.SetGroupResolver(nil); kc.SetTemplateResolver(nil) })
}

// TestKnowledgeCompilerDSL_FixtureDecodesAndBindsParams loads the
// frontend-authored pipeline DSL (agent/templates/compiler.json) and verifies
// it decodes through NewPipelineFromDSL and that the Compiler node's authored
// parameters survive intact. The frontend emits the operator as "Compiler" (its
// canvas label) and does NOT set a "variant" — the variant is derived at runtime
// from the compilation_template's kind. The single fixed operator carries a
// singular compilation_template_group_id (see the frontend
// accordion-operators.tsx restriction and the pipeline.tsx default).
func TestKnowledgeCompilerDSL_FixtureDecodesAndBindsParams(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "compiler.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if _, err := NewPipelineFromDSL(raw, "kc-fixture"); err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	schemas, err := ExtractAllComponentParams(raw)
	if err != nil {
		t.Fatalf("ExtractAllComponentParams: %v", err)
	}
	var compiler *ComponentParamsSchema
	for i := range schemas {
		if schemas[i].ComponentName == "Compiler" {
			compiler = &schemas[i]
			break
		}
	}
	if compiler == nil {
		t.Fatal("fixture has no component with component_name \"Compiler\"")
	}
	params := compiler.ParamsDefaults

	// The frontend Compiler DSL omits "variant"; the Go component derives it
	// from the template kind, so absence is expected.
	if _, ok := params["variant"]; ok {
		t.Errorf("fixture unexpectedly sets variant; frontend Compiler DSL omits it")
	}

	// Authored as a single string group id in the frontend form. The shipped
	// template leaves it empty by design (the user selects the template group at
	// runtime), so we only assert the DSL shape is a plain string (and that the
	// node carries no variant).
	if _, ok := params["compilation_template_group_id"].(string); !ok {
		t.Fatalf("compilation_template_group_id = %v, want a single string id", params["compilation_template_group_id"])
	}
}

// TestKnowledgeCompilerDSL_FrontendDSLDecodesAndConstructs documents the
// contract between the frontend-authored Compiler DSL and the Go component: as
// emitted, the DSL has no "variant" but carries compilation_template_group_id,
// so ParseParam (and thus the runtime factory) succeeds without any server-side
// variant injection. What the component does need at Invoke time is the
// TemplateResolver seam — without it, resolving the template fails loudly
// rather than compiling with no template config.
func TestKnowledgeCompilerDSL_FrontendDSLDecodesAndConstructs(t *testing.T) {
	ctx := t.Context()
	raw, err := os.ReadFile(filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "compiler.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	schemas, err := ExtractAllComponentParams(raw)
	if err != nil {
		t.Fatalf("ExtractAllComponentParams: %v", err)
	}
	var compiler *ComponentParamsSchema
	for i := range schemas {
		if schemas[i].ComponentName == "Compiler" {
			compiler = &schemas[i]
			break
		}
	}
	if compiler == nil {
		t.Fatal("fixture has no component with component_name \"Compiler\"")
	}
	params := compiler.ParamsDefaults

	runtime.InstallDefaultRegistryFactory()
	f := runtime.DefaultFactory()
	if f == nil {
		t.Fatal("default runtime factory not installed")
	}

	// The fixture params carry no variant and a string group id. The shipped
	// template leaves the group id empty (selected at runtime), so give it a
	// concrete id here to verify the DSL constructs once configured.
	params["compilation_template_group_id"] = "tpl-group"
	comp, err := f("Compiler", params)
	if err != nil {
		t.Fatalf("construct from fixture params: %v", err)
	}
	if comp == nil {
		t.Fatal("construct from fixture params: nil component")
	}

	// Without an installed TemplateResolver, the resolution seam fails loud.
	kc.SetTemplateResolver(nil)
	if _, err = kc.ResolveTemplate(ctx, dao.DB, "tenant", "t1"); err == nil {
		t.Fatal("ResolveTemplate without resolver: expected error")
	}
}

// TestKnowledgeCompilerDSL_RegisteredAndConstructible confirms the Go runtime
// registers the knowledge-compiler component under the unified name "Compiler"
// (matching the Python side rag/flow/compiler/compiler.py) and that the runtime
// factory can build a component instance from a DSL params map that carries
// either compilation_template_id or compilation_template_group_id (the variant
// is no longer part of the DSL surface).
func TestKnowledgeCompilerDSL_RegisteredAndConstructible(t *testing.T) {
	runtime.InstallDefaultRegistryFactory()
	if _, _, _, ok := runtime.DefaultRegistry.Lookup("Compiler"); !ok {
		t.Fatal("Compiler not registered in the runtime factory")
	}
	f := runtime.DefaultFactory()
	if f == nil {
		t.Fatal("default runtime factory not installed")
	}

	cases := []map[string]any{
		{"compilation_template_id": "t1"},
		{"compilation_template_group_id": "g1"},
		// Both present: compilation_template_id wins (priority id > group_id).
		{"compilation_template_id": "t1", "compilation_template_group_id": "g1"},
	}
	for i, params := range cases {
		comp, err := f("Compiler", params)
		if err != nil {
			t.Fatalf("case %d construct: %v", i, err)
		}
		if comp == nil {
			t.Fatalf("case %d: nil component", i)
		}
	}

	// Param map with neither id resolves to a parse error.
	if _, err := f("Compiler", map[string]any{}); err == nil {
		t.Fatal("construct with no template spec: expected error")
	}
}

// TestKnowledgeCompilerDSL_ParamBinding exercises the DSL params map ->
// common.Param translation directly, including the id > group_id priority, the
// scalar/extra/default parsing that the pipeline wires from the canvas node
// params, and that the variant is intentionally NOT part of the DSL surface.
func TestKnowledgeCompilerDSL_ParamBinding(t *testing.T) {
	// compilation_template_id wins over compilation_template_group_id.
	p, err := kc.ParseParam(map[string]any{
		"compilation_template_id":       "t1",
		"compilation_template_group_id": "g1",
		"llm_id":                        "llm-1",
		"embedding_model":               "emb-1",
		"tenant_id":                     "tenant-1",
		"dataset_id":                    "kb-1",
		"language":                      "Chinese",
		"extra":                         map[string]any{"prompt": "summarize"},
	})
	if err != nil {
		t.Fatalf("ParseParam: %v", err)
	}
	if p.CompilationTemplateID != "t1" {
		t.Errorf("CompilationTemplateID = %q, want t1", p.CompilationTemplateID)
	}
	if p.CompilationTemplateGroupID != "g1" {
		t.Errorf("CompilationTemplateGroupID = %q, want g1", p.CompilationTemplateGroupID)
	}
	if p.Variant != "" {
		t.Errorf("Variant should be empty after ParseParam (derived from kind later), got %q", p.Variant)
	}
	// TenantID/DatasetID are injected at runtime by the component (not via the
	// DSL/ParseParam), so they are not asserted here.
	if p.LLMID != "llm-1" || p.EmbeddingModel != "emb-1" || p.Language != "Chinese" {
		t.Errorf("scalar fields = %+v", p)
	}
	if p.Extra["prompt"] != "summarize" {
		t.Errorf("Extra = %v, want prompt=summarize", p.Extra)
	}
	// Defaults must be applied for fields the DSL omits.
	if p.MaxWorkers != 4 {
		t.Errorf("MaxWorkers default = %d, want 4", p.MaxWorkers)
	}
	if p.SimilarityThreshold != 0.99 {
		t.Errorf("SimilarityThreshold default = %v, want 0.99", p.SimilarityThreshold)
	}
	if p.EnableHistoricalDedup {
		t.Errorf("EnableHistoricalDedup default = true, want false")
	}
	if p.Plan != nil {
		t.Errorf("Plan = %v, want nil when omitted from DSL", *p.Plan)
	}

	for _, tc := range []struct {
		name string
		plan bool
	}{
		{name: "mode_a", plan: false},
		{name: "mode_b", plan: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := kc.ParseParam(map[string]any{
				"compilation_template_id": "t1",
				"plan":                    tc.plan,
			})
			if err != nil {
				t.Fatalf("ParseParam: %v", err)
			}
			if parsed.Plan == nil || *parsed.Plan != tc.plan {
				t.Fatalf("Plan = %v, want explicit %t", parsed.Plan, tc.plan)
			}
			if parsed.PlanEnabled() != tc.plan {
				t.Errorf("PlanEnabled() = %t, want %t", parsed.PlanEnabled(), tc.plan)
			}
		})
	}
}

// TestKnowledgeCompilerDSL_KindToVariant locks the compilation_template.kind ->
// Go Variant mapping. tree->tree, mind_map->mindmap, wiki->wiki; the
// graph-family kinds (page_index / session_essence / session_graph / timeline /
// knowledge_graph) all collapse onto the structure (graph) variant; unknown
// kinds return ErrUnknownVariant.
func TestKnowledgeCompilerDSL_KindToVariant(t *testing.T) {
	cases := []struct {
		kind string
		want kc.Variant
	}{
		{"tree", kc.VariantTree},
		{"mind_map", kc.VariantMindmap},
		{"wiki", kc.VariantWiki},
		{"page_index", kc.VariantStructure},
		{"session_essence", kc.VariantStructure},
		{"session_graph", kc.VariantStructure},
		{"timeline", kc.VariantStructure},
		{"knowledge_graph", kc.VariantStructure},
	}
	for _, c := range cases {
		got, err := kc.KindToVariant(c.kind)
		if err != nil {
			t.Fatalf("KindToVariant(%q): %v", c.kind, err)
		}
		if got != c.want {
			t.Errorf("KindToVariant(%q) = %q, want %q", c.kind, got, c.want)
		}
	}

	if _, err := kc.KindToVariant("not-a-real-kind"); err == nil {
		t.Fatal("KindToVariant(unknown): expected ErrUnknownVariant")
	}
}

// TestKnowledgeCompilerDSL_VariantValidation documents the parse-time validation
// boundary reachable without a running LLM/embedder: at least one of
// compilation_template_id / compilation_template_group_id is required. The
// variant is not parsed from the DSL at all (it is derived from the resolved
// template's kind at Invoke time).
func TestKnowledgeCompilerDSL_VariantValidation(t *testing.T) {
	if _, err := kc.ParseParam(map[string]any{}); err == nil {
		t.Fatal("ParseParam with no template spec: expected error")
	}
	if _, err := kc.ParseParam(map[string]any{"compilation_template_group_id": ""}); err == nil {
		t.Fatal("ParseParam with empty group id: expected error")
	}
	if _, err := kc.ParseParam(map[string]any{"compilation_template_id": "t1"}); err != nil {
		t.Fatalf("ParseParam with id only: unexpected error %v", err)
	}
}

// TestKnowledgeCompilerDSL_ResolveTemplateSeam verifies the group->template
// resolution path uses the installed seams: compilation_template_group_id
// resolves to child template ids (GroupResolver) and each is loaded by
// TemplateResolver, which yields the kind that selects the variant.
func TestKnowledgeCompilerDSL_ResolveTemplateSeam(t *testing.T) {
	installStubResolvers(t, map[string][]string{
		"g1": {"t1", "t2"},
	}, map[string]kc.TemplateInfo{
		"t1": {ID: "t1", Kind: "mind_map", Config: map[string]any{"language": "English"}},
		"t2": {ID: "t2", Kind: "tree", Config: map[string]any{}},
	})

	ctx := t.Context()

	ids, err := kc.ResolveGroupTemplateIDs(ctx, dao.DB, "tenant", []string{"g1"})
	if err != nil {
		t.Fatalf("ResolveGroupTemplateIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "t1" || ids[1] != "t2" {
		t.Fatalf("group resolved to %v, want [t1 t2]", ids)
	}

	info, err := kc.ResolveTemplate(ctx, dao.DB, "tenant", "t1")
	if err != nil {
		t.Fatalf("ResolveTemplate t1: %v", err)
	}
	if info.Kind != "mind_map" {
		t.Errorf("template kind = %q, want mind_map", info.Kind)
	}
	if v, _ := kc.KindToVariant(info.Kind); v != kc.VariantMindmap {
		t.Errorf("kind %q -> variant %q, want mindmap", info.Kind, v)
	}
}
