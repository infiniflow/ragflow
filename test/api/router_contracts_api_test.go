//go:build integration

package api

import (
	"fmt"
	"net/http"
	"testing"
)

// requireRouteNotFound asserts the HandleNoRoute JSON contract for a request
// whose path (relative to the API base) is path. The query string must not
// appear in the message.
func requireRouteNotFound(t *testing.T, r restResult, path string) {
	t.Helper()

	restCode(t, r, http.StatusNotFound, 404)
	if r.JSON["error"] != "Not Found" {
		t.Fatalf("expected error %q, got %v, payload: %v", "Not Found", r.JSON["error"], r.JSON)
	}
	want := fmt.Sprintf("Not Found: /api/%s%s", TestConfig.APIVersion, path)
	if r.JSON["message"] != want {
		t.Fatalf("expected message %q, got %v, payload: %v", want, r.JSON["message"], r.JSON)
	}
	data, ok := r.JSON["data"]
	if !ok || data != nil {
		t.Fatalf("expected data null, got %v, payload: %v", r.JSON["data"], r.JSON)
	}
}

func TestRouteNotFoundReturnsJSON(t *testing.T) {
	anonymous := *TestConfig
	anonymous.APIKey = ""
	requireRouteNotFound(t, restCall(t, &anonymous, http.MethodGet, "/__missing_route__", nil, nil), "/__missing_route__")
}
