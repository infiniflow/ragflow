package common

import "testing"

func TestParseParamThinkingSettings(t *testing.T) {
	for _, thinking := range []string{"", "default", "disabled", "enabled"} {
		t.Run(thinking, func(t *testing.T) {
			p, err := ParseParam(map[string]any{
				"compilation_template_id": "template-1",
				"thinking":                thinking,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !p.ChatSettings.ThinkingSet || p.ChatSettings.Thinking == nil {
				t.Fatal("thinking choice must set an explicit override")
			}
			if got, want := *p.ChatSettings.Thinking, thinking == "enabled"; got != want {
				t.Errorf("thinking = %v, want %v", got, want)
			}
		})
	}
}

// TestKindToVariant_WhitelistCoversAllValidKinds covers O2a: every whitelisted
// template kind maps to a non-empty variant without error.
func TestKindToVariant_WhitelistCoversAllValidKinds(t *testing.T) {
	validKinds := []string{
		"tree", "mind_map", "mindmap", "wiki", "page_index",
		"session_essence", "session_graph",
		"knowledge_graph", "structure", "knowledgegraph", "graph",
	}
	for _, kind := range validKinds {
		v, err := KindToVariant(kind)
		if err != nil {
			t.Errorf("KindToVariant(%q) unexpected error: %v", kind, err)
		}
		if v == "" {
			t.Errorf("KindToVariant(%q) returned empty variant", kind)
		}
	}
}

// TestKindToVariant_UnknownKindHardFails covers O2a: a kind outside the
// whitelist returns an error (no fallback) so the rebuild/component path cannot
// silently mis-route an unknown template.
func TestKindToVariant_UnknownKindHardFails(t *testing.T) {
	for _, kind := range []string{"garbage", "artifact_page", "unknown_xyz", ""} {
		if _, err := KindToVariant(kind); err == nil {
			t.Errorf("KindToVariant(%q) should hard-fail (O2a), got nil error", kind)
		}
	}
}

// TestKindToVariant_CanonicalForms covers O2a aliases: the same template kind
// in different canonical spellings maps to the same variant.
func TestKindToVariant_CanonicalForms(t *testing.T) {
	a, err := KindToVariant("structure")
	if err != nil {
		t.Fatalf("structure: %v", err)
	}
	b, err := KindToVariant("knowledge_graph")
	if err != nil {
		t.Fatalf("knowledge_graph: %v", err)
	}
	c, err := KindToVariant("graph")
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if a != b || a != c {
		t.Errorf("structure/knowledge_graph/graph should map to the same variant, got %q %q %q", a, b, c)
	}
}

func TestKindToTaskTypePreservesFrontendCategories(t *testing.T) {
	tests := map[string]string{
		"wiki":            TaskTypeWiki,
		"tree":            TaskTypeTree,
		"knowledge_graph": TaskTypeGraph,
		"mind_map":        TaskTypeMindmap,
		"page_index":      TaskTypePageIndex,
	}
	for kind, want := range tests {
		got, err := KindToTaskType(kind)
		if err != nil {
			t.Errorf("KindToTaskType(%q) unexpected error: %v", kind, err)
			continue
		}
		if got != want {
			t.Errorf("KindToTaskType(%q) = %q, want %q", kind, got, want)
		}
	}
}
