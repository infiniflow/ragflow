//go:build integration

package api

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestRESTPluginTools(t *testing.T) {
	for _, key := range []string{"", "invalid-api-key"} {
		c := *TestConfig
		c.APIKey = key
		restCode(t, restCall(t, &c, http.MethodGet, "/plugin/tools", nil, nil), 401, 401)
	}
	var previous []any
	for range 2 {
		result := restCall(t, TestConfig, http.MethodGet, "/plugin/tools", nil, nil)
		tools := array(t, restOK(t, result)["data"])
		if len(tools) == 0 {
			t.Fatal("embedded tool catalog is empty")
		}
		names := map[string]bool{}
		for _, value := range tools {
			tool := object(t, value)
			for _, field := range []string{"name", "displayName", "description", "displayDescription"} {
				text, ok := tool[field].(string)
				if !ok || strings.TrimSpace(text) == "" {
					t.Fatalf("tool has invalid %s", field)
				}
			}
			name := tool["name"].(string)
			if names[name] {
				t.Fatalf("duplicate tool name %q", name)
			}
			names[name] = true
			for name, value := range object(t, tool["parameters"]) {
				if name == "" {
					t.Fatal("parameter has empty name")
				}
				parameter := object(t, value)
				for _, field := range []string{"type", "description", "displayDescription"} {
					text, ok := parameter[field].(string)
					if !ok || strings.TrimSpace(text) == "" {
						t.Fatalf("parameter %q has invalid %s", name, field)
					}
				}
				if _, ok := parameter["required"].(bool); !ok {
					t.Fatalf("parameter %q lacks boolean required flag", name)
				}
			}
		}
		if previous != nil && !reflect.DeepEqual(previous, tools) {
			t.Fatal("read-only plugin listing changed between requests")
		}
		previous = tools
	}
}
