package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWikiMapPromptClaimGuidance(t *testing.T) {
	tests := []struct {
		name   string
		fields []any
		want   []string
	}{
		{
			name: "canonical fields",
			fields: []any{
				map[string]any{"statement": "  Extract only audited financial claims.  ", "subject": "Use the legal entity name."},
				map[string]any{"statement": "Keep the reporting period.", "subject": "Do not use department aliases."},
			},
			want: []string{
				"- Claim statement guidance: Extract only audited financial claims.\nKeep the reporting period.",
				"- Claim subject guidance: Use the legal entity name.\nDo not use department aliases.",
			},
		},
		{
			name:   "missing subject retains its default",
			fields: []any{map[string]any{"statement": "Extract only dated claims."}},
			want:   []string{"- Claim statement guidance: Extract only dated claims.", "- Claim subject guidance: entity or concept that the claim is about"},
		},
		{
			name:   "empty fields retain defaults",
			fields: []any{},
			want:   []string{"- Claim statement guidance: factual statement", "- Claim subject guidance: entity or concept that the claim is about"},
		},
		{
			name: "blank fields retain defaults",
			fields: []any{
				map[string]any{"statement": " \n ", "subject": ""},
				nil,
			},
			want: []string{"- Claim statement guidance: factual statement", "- Claim subject guidance: entity or concept that the claim is about"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := map[string]any{"claim": map[string]any{"fields": tt.fields}}
			prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, "English")
			for _, want := range tt.want {
				if !strings.Contains(prompt, want) {
					t.Errorf("MAP prompt missing claim guidance %q", want)
				}
			}
		})
	}
}

func TestWikiMapPromptConceptGuidance(t *testing.T) {
	cfg := map[string]any{"concept": map[string]any{"fields": []any{
		map[string]any{"term": "  Audit  ", "definition_excerpt": "Independent review"},
		map[string]any{"term": "Revenue", "definition_excerpt": "Sales income"},
	}}}
	prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, "English")
	for _, want := range []string{
		"- Concept term guidance: Audit|Revenue",
		"- Concept definition guidance: Audit:Independent review\nRevenue:Sales income",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("MAP prompt missing concept guidance %q", want)
		}
	}
}

func TestWikiMapPromptUsesBuiltinClaimGuidance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "knowledge_compile", "templates", "wiki.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		Config map[string]any `yaml:"config"`
	}
	if err := yaml.Unmarshal(data, &template); err != nil {
		t.Fatal(err)
	}
	fields := wikiTemplateFields(template.Config, "claim")
	if len(fields) == 0 {
		t.Fatal("builtin wiki template has no claim fields")
	}
	prompt, _ := buildWikiMapPrompt("doc-1", nil, template.Config, "English")
	for _, key := range []string{"statement", "subject"} {
		guidance, ok := fields[0].(map[string]any)[key].(string)
		if !ok || strings.TrimSpace(guidance) == "" {
			t.Fatalf("builtin claim has no %s guidance", key)
		}
		if !strings.Contains(prompt, strings.TrimSpace(guidance)) {
			t.Errorf("MAP prompt missing builtin claim %s guidance %q", key, guidance)
		}
	}
}
