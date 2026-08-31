//go:build !cgo

package infnative

import "fmt"

// DefaultDropScore mirrors the cgo implementation's default (0.5).
// Defined here so cmd/ragflow_server.go can compile without cgo.
const DefaultDropScore = 0.5

// Register is a stub for non-cgo builds. It always returns an error so
// registerNativeDeepDoc can warn and fallback to MockDocAnalyzer.
func Register(modelDir string, dropScore float64) error {
	return fmt.Errorf("in-process DeepDoc unavailable: built without cgo (hint: build with bash build.sh --all)")
}

// Serving reports false for non-cgo builds.
func Serving() bool {
	return false
}
