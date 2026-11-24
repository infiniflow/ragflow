//go:build !ee

package service

import (
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestCommunityBuiltinTemplates(t *testing.T) {
	t.Chdir("../..")
	templates, err := loadBuiltinTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatal("shared builtin templates missing")
	}
	for _, template := range templates {
		if _, err := kccommon.KindToVariant(template.Kind); err != nil {
			t.Errorf("unsupported builtin %q: %v", template.Kind, err)
		}
		if template.Kind == "timeline" {
			t.Fatal("enterprise timeline exposed in community palette")
		}
	}
}
