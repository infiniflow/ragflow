//go:build integration

package api

import (
	"net/http"
	"testing"
)

const langfuseAPIKeyPath = "/langfuse/api-key"

// requireLangfuseMissingFields asserts the service-layer validation response for
// a request that omits at least one required Langfuse field.
func requireLangfuseMissingFields(t *testing.T, r restResult) {
	t.Helper()

	restCode(t, r, http.StatusOK, 102)
	if r.JSON["message"] != "missing required fields" {
		t.Fatalf("expected message %q, got %v, payload: %v", "missing required fields", r.JSON["message"], r.JSON)
	}
}

func TestLangfuseAPIKeyRoutesRequireAuth(t *testing.T) {
	body := apiObject{
		"secret_key": "s",
		"public_key": "p",
		"host":       "http://example.com",
	}

	cases := []struct {
		name   string
		method string
		body   apiObject
	}{
		{name: "GET", method: http.MethodGet, body: nil},
		{name: "POST", method: http.MethodPost, body: body},
		{name: "PUT", method: http.MethodPut, body: body},
		{name: "DELETE", method: http.MethodDelete, body: nil},
	}

	anonymous := *TestConfig
	anonymous.APIKey = ""

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := restCall(t, &anonymous, tc.method, langfuseAPIKeyPath, tc.body, nil)
			restCode(t, r, http.StatusUnauthorized, 401)
			if r.JSON["message"] != "Missing Authorization header" {
				t.Fatalf("expected message %q, got %v, payload: %v", "Missing Authorization header", r.JSON["message"], r.JSON)
			}
		})
	}
}

func TestLangfuseAPIKeyMissingRequiredFields(t *testing.T) {
	r := restCall(t, TestConfig, http.MethodPost, langfuseAPIKeyPath, apiObject{
		"secret_key": "",
		"public_key": "pub",
		"host":       "http://host",
	}, nil)
	requireLangfuseMissingFields(t, r)
}
