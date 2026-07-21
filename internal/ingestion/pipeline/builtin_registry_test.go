package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryLoadsSummaries(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("ingestion_pipeline_general.json", `{"title":{"zh":"通用","en":"General"},"description":{"zh":"desc"},"dsl":{"components":{}}}`)
	mustWrite("ingestion_pipeline_book.json", `{"title":{"en":"Book"},"dsl":{"components":{}}}`)

	r, err := NewRegistryFromDir(dir)
	if err != nil {
		t.Fatalf("NewRegistryFromDir: %v", err)
	}
	if got := len(r.List().BuiltinPipelines); got != 2 {
		t.Fatalf("len(List()) = %d, want 2", got)
	}
	if got := r.List().Total; got != 2 {
		t.Fatalf("Total = %d, want 2", got)
	}
	tpl, ok := r.Get("general")
	if !ok {
		t.Fatal("expected general template")
	}
	// Title must be the English value so it aligns with the front-end's
	// hardcoded parser labels, not a localized variant.
	if tpl.Title != "General" {
		t.Fatalf("title = %q, want General", tpl.Title)
	}
	if tpl.Description != "desc" {
		t.Fatalf("description = %q, want desc (English)", tpl.Description)
	}
}

func TestRegistryRefsMatchList(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("ingestion_pipeline_general.json", `{"title":{"en":"General"},"dsl":{"components":{}}}`)
	mustWrite("ingestion_pipeline_book.json", `{"title":{"en":"Book"},"dsl":{"components":{}}}`)

	r, err := NewRegistryFromDir(dir)
	if err != nil {
		t.Fatalf("NewRegistryFromDir: %v", err)
	}
	refs := r.Refs()
	if len(refs) != 2 {
		t.Fatalf("len(Refs()) = %d, want 2", len(refs))
	}
	if refs[0] != "book" || refs[1] != "general" {
		t.Fatalf("refs = %#v, want sorted builtin refs", refs)
	}
}

// TestRegistryVsHardcodedList compares the built-in registry with the hardcoded list in service/dataset.go
func TestRegistryVsHardcodedList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}

	registryRefs := make(map[string]bool)
	for _, ref := range r.Refs() {
		registryRefs[ref] = true
		t.Logf("Registry has: %s", ref)
	}

	// Hardcoded list from service/dataset.go
	hardcoded := map[string]bool{
		"naive":        true,
		"book":         true,
		"email":        true,
		"laws":         true,
		"manual":       true,
		"one":          true,
		"paper":        true,
		"picture":      true,
		"presentation": true,
		"qa":           true,
		"resume":       true,
		"table":        true,
		"tag":          true,
	}
	for h := range hardcoded {
		t.Logf("Hardcoded has: %s", h)
	}

	// Find differences
	onlyInRegistry := []string{}
	onlyInHardcoded := []string{}
	inBoth := []string{}

	for ref := range registryRefs {
		if hardcoded[ref] {
			inBoth = append(inBoth, ref)
		} else {
			onlyInRegistry = append(onlyInRegistry, ref)
		}
	}
	for h := range hardcoded {
		if !registryRefs[h] {
			onlyInHardcoded = append(onlyInHardcoded, h)
		}
	}

	t.Logf("--- Summary ---")
	t.Logf("In both: %v", inBoth)
	t.Logf("Only in registry: %v", onlyInRegistry)
	t.Logf("Only in hardcoded: %v", onlyInHardcoded)

	// After alias mechanism: naive is an alias for general, so every
	// hardcoded value is now reachable through the registry.
	for h := range hardcoded {
		if !r.IsValid(h) {
			t.Errorf("hardcoded value %q is not valid in registry (not a ref and not an alias)", h)
		}
	}
}

// TestRegistryAliasNaiveResolvesGeneral verifies that the legacy parser_id
// "naive" resolves to the "general" builtin template via the alias mechanism.
// This keeps existing dataset rows (which store parser_id="naive") runnable
// after the hardcoded list is removed.
func TestRegistryAliasNaiveResolvesGeneral(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}

	// "naive" is valid (alias) even though it is not in Refs().
	if !r.IsValid("naive") {
		t.Error("IsValid(naive) = false, want true")
	}
	if !r.IsValid("general") {
		t.Error("IsValid(general) = false, want true")
	}
	if r.IsValid("unknown-parser") {
		t.Error("IsValid(unknown-parser) = true, want false")
	}

	// Get("naive") returns the general template.
	tpl, ok := r.Get("naive")
	if !ok {
		t.Fatal("Get(naive) returned false, want true")
	}
	if tpl.ParserID != "general" {
		t.Fatalf("Get(naive).ParserID = %q, want general", tpl.ParserID)
	}
	if tpl.Filename != "ingestion_pipeline_general.json" {
		t.Fatalf("Get(naive).Filename = %q, want ingestion_pipeline_general.json", tpl.Filename)
	}

	// Refs/List must NOT include the alias - only canonical templates.
	for _, ref := range r.Refs() {
		if ref == "naive" {
			t.Error("Refs() contains alias naive; aliases must be hidden from listing")
		}
	}
	for _, meta := range r.List().BuiltinPipelines {
		if meta.ParserID == "naive" {
			t.Error("List() contains alias naive; aliases must be hidden from listing")
		}
	}
}

// TestLoadBuiltinDSL verifies that valid and aliased parser_ids resolve to
// real DSL content, and that unknown ids fail.
func TestLoadBuiltinDSL_Canonical(t *testing.T) {
	dsl, err := LoadBuiltinDSL("general")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL(general): %v", err)
	}
	if dsl == "" {
		t.Fatal("LoadBuiltinDSL(general) returned empty DSL")
	}
	// DSL should start like a JSON object containing the canvas components.
	if dsl[0] != '{' {
		t.Fatalf("dsl is not JSON: %s", dsl[:80])
	}
}

func TestLoadBuiltinDSL_NaiveAlias(t *testing.T) {
	generalDSL, err := LoadBuiltinDSL("general")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL(general): %v", err)
	}
	naiveDSL, err := LoadBuiltinDSL("naive")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL(naive): %v", err)
	}
	if generalDSL != naiveDSL {
		t.Fatal("naive alias must resolve to general DSL")
	}
}

func TestRegistryMetaIncludesDSL(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("ingestion_pipeline_general.json", `{
		"title": {"en": "General"},
		"dsl": {"components": {"File": {"obj": {"component_name": "File"}}}}
	}`)

	r, err := NewRegistryFromDir(dir)
	if err != nil {
		t.Fatalf("NewRegistryFromDir: %v", err)
	}

	general, ok := r.Get("general")
	if !ok {
		t.Fatal("Get(general) failed")
	}

	// Verify DSL is included in the meta.
	if general.DSL == nil {
		t.Fatal("DSL is nil, want non-nil")
	}
	if _, ok := general.DSL["components"]; !ok {
		t.Fatal("DSL missing components key")
	}
}

func TestLoadBuiltinDSL_UnknownFails(t *testing.T) {
	_, err := LoadBuiltinDSL("nonexistent")
	if err == nil {
		t.Fatal("LoadBuiltinDSL(nonexistent) = nil, want error")
	}
}
