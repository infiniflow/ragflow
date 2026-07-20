package component

import "strings"

// stringParam coerces a JSON-decoded parameter value to string.
func stringParam(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

// anySlice coerces a JSON-decoded value to []any.
func anySlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// agentProviderLastSegmentSplit splits a model ID into model name and provider
// name. The convention is "model_name" (tenant-assigned name) where the
// provider was already resolved through the model locator.
func agentProviderLastSegmentSplit(id string) (model, provider string, ok bool) {
	if id == "" {
		return "", "", false
	}
	parts := strings.Split(id, ":")
	// Expected format: "model_name" or "provider:model_name"
	switch len(parts) {
	case 1:
		return parts[0], "", true
	case 2:
		return parts[1], parts[0], true
	default:
		return parts[len(parts)-1], strings.Join(parts[:len(parts)-1], ":"), true
	}
}
