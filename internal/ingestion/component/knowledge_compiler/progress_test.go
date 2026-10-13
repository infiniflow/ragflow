package knowledge_compiler

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

type compilerFractions struct {
	mu     sync.Mutex
	values []float64
}

func (r *compilerFractions) context(ctx context.Context, t *testing.T) context.Context {
	return runtime.BindComponentFraction(runtime.WithProgressFractionCallback(ctx, func(id string, f float64) {
		if id != "Compiler:test" {
			t.Errorf("fraction attributed to %q", id)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.values = append(r.values, f)
	}), "Compiler:test")
}

func (r *compilerFractions) snapshot() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]float64(nil), r.values...)
}

func compilerProgressInputs() map[string]any {
	return map[string]any{
		"tenant_id": "t1", "dataset_id": "ds1", "doc_id": "d1",
		"parser_config": graphParserConfig(),
		"chunks": []any{
			map[string]any{"id": "c1", "text": "Alpha is linked to Beta"},
			map[string]any{"id": "c2", "text": "Beta is linked to Gamma"},
		},
	}
}

func TestCompilerProgressAdvancesWithinTemplates(t *testing.T) {
	for _, variant := range []string{"structure", "wiki", "mindmap", "tree"} {
		t.Run(variant, func(t *testing.T) {
			if variant == "structure" {
				installMockDeps(t)
			} else {
				installProseDeps(t)
			}
			installVariantTemplateResolver(t, variant)
			c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{"compilation_template_id": "tpl-" + variant})
			if err != nil {
				t.Fatal(err)
			}
			r := &compilerFractions{}
			_, err = c.Invoke(r.context(t.Context(), t), nil, compilerProgressInputs())
			if err != nil {
				t.Fatal(err)
			}
			values := r.snapshot()
			intermediate := false
			for _, f := range values {
				if f > 0 && f < 1 {
					intermediate = true
				}
			}
			if !intermediate {
				t.Fatalf("no intermediate progress: %v", values)
			}
			if len(values) == 0 || values[len(values)-1] != 1 {
				t.Fatalf("successful compiler did not finish progress: %v", values)
			}
		})
	}
}

func TestCompilerProgressReservesBudgetForLaterTemplates(t *testing.T) {
	r := &compilerFractions{}
	calls := 0
	common.SetDepsResolver(func(_ context.Context, tenant, _, _ string) (common.Deps, error) {
		calls++
		if calls == 2 {
			values := r.snapshot()
			if len(values) == 0 {
				t.Error("first template did not advance progress")
			}
			for _, f := range values {
				if f <= 0 || f >= 0.5 {
					t.Errorf("first template consumed later budget: %v", values)
					break
				}
			}
		}
		return common.Deps{Chat: proseChat{}, Embed: mockEmbedder{dim: 8}, TenantID: tenant}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })
	common.SetGroupResolver(func(context.Context, *gorm.DB, string, []string) ([]string, error) {
		return []string{"tpl-mindmap", "tpl-mindmap"}, nil
	})
	t.Cleanup(func() { common.SetGroupResolver(testGroupResolver) })
	installVariantTemplateResolver(t, "mindmap")
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{"compilation_template_group_id": "group"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Invoke(r.context(t.Context(), t), nil, compilerProgressInputs()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("ran %d templates", calls)
	}
}

type failedProgressEmbedder struct{ cancel context.CancelFunc }

func (failedProgressEmbedder) Dimensions() int { return 8 }
func (e failedProgressEmbedder) Encode(context.Context, []string) ([][]float32, error) {
	if e.cancel != nil {
		e.cancel()
		return nil, context.Canceled
	}
	return nil, errors.New("embedding failed")
}

func TestCompilerProgressDoesNotFinishOnFailureOrCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "cancellation"}[canceled], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			embed := failedProgressEmbedder{}
			if canceled {
				embed.cancel = cancel
			}
			common.SetDepsResolver(func(context.Context, string, string, string) (common.Deps, error) {
				return common.Deps{Chat: proseChat{}, Embed: embed}, nil
			})
			t.Cleanup(func() { common.SetDepsResolver(nil) })
			installVariantTemplateResolver(t, "mindmap")
			c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{"compilation_template_id": "tpl-mindmap"})
			if err != nil {
				t.Fatal(err)
			}
			r := &compilerFractions{}
			if _, err := c.Invoke(r.context(ctx, t), nil, compilerProgressInputs()); err == nil {
				t.Fatal("expected embedding error")
			}
			values := r.snapshot()
			if len(values) == 0 {
				t.Fatal("completed generation did not advance progress")
			}
			for _, f := range values {
				if f >= 1 {
					t.Fatalf("unfinished compiler reported full progress: %v", values)
				}
			}
		})
	}
}
