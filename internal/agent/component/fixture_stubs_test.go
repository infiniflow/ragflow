package component_test

import (
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component"
)

func TestCompileRejectsAnswerComponent(t *testing.T) {
	c := &canvas.Canvas{
		Components: map[string]canvas.CanvasComponent{
			"begin": {
				Obj:        canvas.CanvasComponentObj{ComponentName: "Begin"},
				Downstream: []string{"answer"},
			},
			"answer": {
				Obj:      canvas.CanvasComponentObj{ComponentName: "Answer"},
				Upstream: []string{"begin"},
			},
		},
	}

	_, err := canvas.Compile(t.Context(), c)
	if err == nil {
		t.Fatal("Compile accepted unsupported Answer component")
	}
	if !strings.Contains(err.Error(), `unknown component "Answer"`) {
		t.Fatalf("Compile error = %q, want unknown Answer component", err)
	}
}
