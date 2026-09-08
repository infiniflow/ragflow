package common

import (
	"fmt"
	"strings"
)

// This file holds the parser_config accessors shared by the structure and
// wiki variants. They mirror Python's structure.py helpers (_struct_get,
// _struct_localize), which wiki.py also imports — one code path in Go as well.

// Get mirrors Python's _struct_get: case-insensitive lookup against the first
// matching key. Returns nil when no key is present.
func Get(cfg map[string]any, keys ...string) any {
	if cfg == nil {
		return nil
	}
	for _, k := range keys {
		if v, ok := cfg[k]; ok {
			return v
		}
		kl := strings.ToLower(k)
		for ck, v := range cfg {
			if strings.ToLower(ck) == kl {
				return v
			}
		}
	}
	return nil
}

// GetMap is Get narrowed to a map value (nil when absent or not a map).
func GetMap(cfg map[string]any, key string) map[string]any {
	if m, ok := Get(cfg, key).(map[string]any); ok {
		return m
	}
	return nil
}

// Localize resolves a multilingual config value to the requested language.
// Mirrors Python's _struct_localize:
//   - string              → returned as-is
//   - []string / []any    → numbered lines ("1. item\n2. item")
//   - map[string]string   → value for lang, else "en" (only when lang != "en")
//   - map[string]any      → lang then "en", each recursively localized
//
// There is deliberately no arbitrary-first-value fallback: Python returns ""
// when neither the requested language nor "en" is present, and silently
// picking an unrelated language would drift the rendered prompts.
func Localize(value any, lang string) string {
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return numberedJoin(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return numberedJoin(parts)
	case map[string]string:
		if s, ok := v[lang]; ok {
			return s
		}
		if lang != "en" {
			return v["en"]
		}
		return ""
	case map[string]any:
		if raw, ok := v[lang]; ok {
			if out := Localize(raw, lang); out != "" {
				return out
			}
		}
		if lang != "en" {
			if raw, ok := v["en"]; ok {
				return Localize(raw, "en")
			}
		}
		return ""
	default:
		return ""
	}
}

// numberedJoin renders a list config value the way Python's _struct_localize
// does: "1. first\n2. second\n...".
func numberedJoin(items []string) string {
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d. %s", i+1, item)
	}
	return b.String()
}
