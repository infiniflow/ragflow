//go:build !ee

package structure

import (
	"errors"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestUnsupportedTemplateKind(t *testing.T) {
	for _, kind := range []string{"timeline", "unknown"} {
		_, err := Run(t.Context(), common.Deps{}, testParam(), common.Inputs{
			VariantSpecific: map[string]any{"parser_config": map[string]any{"kind": kind}},
		})
		if !errors.Is(err, common.ErrUnknownVariant) {
			t.Errorf("Run(kind=%q) error = %v", kind, err)
		}
	}
}
