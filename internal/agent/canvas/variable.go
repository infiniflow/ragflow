// Package canvas — variable reference helpers (re-exports).
//
// The canonical VarRefPattern / ExtractRefs / ResolveTemplate
// implementations live in internal/agent/runtime/template.go so
// components can depend on them without importing canvas. This file
// re-exports the symbols for callers that already use canvas.X.
package canvas

import (
	"ragflow/internal/agent/runtime"
)

// VarRefPattern aliases runtime.VarRefPattern.
var VarRefPattern = runtime.VarRefPattern

// ExtractRefs re-exports runtime.ExtractRefs.
func ExtractRefs(s string) []string {
	return runtime.ExtractRefs(s)
}

// ResolveTemplate re-exports runtime.ResolveTemplate.
func ResolveTemplate(s string, state *CanvasState) (string, error) {
	return runtime.ResolveTemplate(s, state)
}
