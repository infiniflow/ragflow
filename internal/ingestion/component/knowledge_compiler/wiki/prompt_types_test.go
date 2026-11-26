package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWikiMapPromptCanonicalTypeGuidance(t *testing.T) {
	cfg := map[string]any{
		"entity": map[string]any{"fields": []any{map[string]any{
			"type": "sensor", "description": "An installed physical sensor.", "rule": "Keep its exact commissioning identifier.",
		}}},
		"relation": map[string]any{"fields": []any{map[string]any{
			"type": "calibrates", "description": "Calibration performed on equipment.", "rule": "Direction is technician to sensor.",
		}}},
	}
	prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, "English")
	for _, want := range []string{"sensor", "An installed physical sensor.", "Keep its exact commissioning identifier.", "calibrates", "Calibration performed on equipment.", "Direction is technician to sensor."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("MAP prompt missing saved type guidance %q", want)
		}
	}
	for _, want := range []string{`"name": "string"`, `"from": "string"`, `"to": "string"`, `"source_chunk_id": "string"`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("MAP output contract lost %q", want)
		}
	}
}

func TestWikiMapPromptBuiltinTypeRules(t *testing.T) {
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
	prompt, _ := buildWikiMapPrompt("doc-1", nil, template.Config, "English")
	for _, section := range []string{"entity", "relation"} {
		for _, raw := range wikiTemplateFields(template.Config, section) {
			field := raw.(map[string]any)
			for _, key := range []string{"description", "rule"} {
				if want := strings.TrimSpace(field[key].(string)); !strings.Contains(prompt, want) {
					t.Errorf("MAP prompt missing builtin %s %s: %q", section, key, want)
				}
			}
		}
	}
}

func TestWikiMapPromptTypeGuidanceDefaults(t *testing.T) {
	for _, fields := range [][]any{nil, {}, {nil, map[string]any{"type": "  ", "description": "ignored"}}} {
		cfg := map[string]any{"entity": map[string]any{"fields": fields}, "relation": map[string]any{"fields": fields}}
		prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, "English")
		for _, want := range []string{
			"- Entity types (override default): person|org|product|regulation|location|system|equipment|other",
			"- Relation types (override default): include|ordered|owns|part_of|caused_by|regulates|uses|located_in|other",
		} {
			if !strings.Contains(prompt, want) {
				t.Errorf("default type guidance lost %q", want)
			}
		}
	}
}

func TestWikiMapPromptTypeGuidanceOrderAndTypedFields(t *testing.T) {
	cfg := map[string]any{"entity": map[string]any{"fields": []map[string]any{
		{"type": " sensor ", "description": " Device. ", "rule": " Keep ID. "},
		{"type": "technician", "description": "Qualified operator."},
	}}}
	prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, "English")
	if want := "- Entity types (override default): sensor: Device.\nKeep ID.\ntechnician: Qualified operator."; !strings.Contains(prompt, want) {
		t.Errorf("typed fields lost their order or guidance: %q", want)
	}
}

func TestWikiMapPromptTypeGuidanceLocalization(t *testing.T) {
	cfg := map[string]any{"entity": map[string]any{"fields": []any{
		map[string]any{"type": "sensor", "description": map[string]any{"zh": "Installed sensor.", "en": "English description."}, "rule": map[string]any{"en": "Keep the sensor ID."}},
		map[string]any{"type": "technician", "description": map[string]any{"fr": "Do not select an unrelated language."}, "rule": "  "},
	}}}
	for _, language := range []string{"zh", "Chinese", " chinese "} {
		t.Run(language, func(t *testing.T) {
			prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, language)
			if want := "sensor: Installed sensor.\nKeep the sensor ID.\ntechnician"; !strings.Contains(prompt, want) {
				t.Errorf("localized type guidance missing %q", want)
			}
			if strings.Contains(prompt, "English description.") || strings.Contains(prompt, "Do not select an unrelated language.") {
				t.Error("selected wrong-language guidance")
			}
		})
	}
}

func TestWikiMapPromptTypeGuidanceUnknownLanguage(t *testing.T) {
	cfg := map[string]any{"entity": map[string]any{"fields": []any{
		map[string]any{"type": "sensor", "description": map[string]any{"custom-language": "Custom guidance.", "en": "English fallback."}, "rule": map[string]any{"en": "Fallback rule."}},
	}}}
	prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, "custom-language")
	if !strings.Contains(prompt, "sensor: Custom guidance.\nFallback rule.") || strings.Contains(prompt, "English fallback.") {
		t.Error("unknown language key or English fallback was lost")
	}
}

func TestWikiMapPromptTypeGuidancePreservesGlobalLanguage(t *testing.T) {
	cfg := map[string]any{
		"global_rules": map[string]any{"Chinese": "Global display-name rules.", "zh": "Global code rules."},
		"entity": map[string]any{"fields": []any{map[string]any{
			"type": "sensor", "description": map[string]any{"zh": "Chinese sensor.", "en": "English sensor."},
		}}},
	}
	for _, tc := range []struct{ language, want, reject string }{
		{"Chinese", "Global display-name rules.", "Global code rules."},
		{"zh", "Global code rules.", "Global display-name rules."},
	} {
		prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, tc.language)
		if !strings.Contains(prompt, "sensor: Chinese sensor.") || !strings.Contains(prompt, tc.want) || strings.Contains(prompt, tc.reject) {
			t.Errorf("type language normalization changed global-rule selection for %q", tc.language)
		}
	}
	for _, language := range []string{"en", "English", " english "} {
		prompt, _ := buildWikiMapPrompt("doc-1", nil, cfg, language)
		if !strings.Contains(prompt, "sensor: English sensor.") || strings.Contains(prompt, "Chinese sensor.") {
			t.Errorf("language %q selected wrong type guidance", language)
		}
	}
}
