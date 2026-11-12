//go:build !ee

package observability

func BeginRetrieval(_ string) func(*error) { return func(*error) {} }
