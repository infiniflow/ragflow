package common

import (
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestAgentEventSnapshotBoundsAndRedactsWithoutMutation(t *testing.T) {
	credentials := map[string]any{"password": "keep-secret", "api_key": "keep-key", "authorization": "Bearer keep-auth", "client_secret": "keep-client"}
	input := map[string]any{"nested": credentials, "file": "data:image/png;base64," + strings.Repeat("binary", 2000), "raw": []byte("binary"), "state": map[string]any{"secret": "state-secret"}, "result": strings.Repeat("result", 5000)}
	snapshot, truncated := AgentEventSnapshot(input)
	if !truncated || len(snapshot) > 16384 {
		t.Fatalf("unbounded snapshot: %d, %v", len(snapshot), truncated)
	}
	for _, secret := range []string{"keep-secret", "keep-key", "keep-auth", "keep-client", "base64", "binarybinary", "state-secret"} {
		if strings.Contains(snapshot, secret) {
			t.Fatalf("payload includes %q", secret)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(snapshot), &decoded); err != nil {
		t.Fatalf("invalid bounded JSON: %v", err)
	}
	if credentials["password"] != "keep-secret" {
		t.Fatal("snapshot mutated component input")
	}
}

func TestAgentEventSnapshotRedactsEncodedHeadersAndURLs(t *testing.T) {
	input := map[string]any{"headers": `{"Authorization":"Bearer header-secret"}`, "body": `{"nested":{"password":"body-secret"}}`, "url": "https://user:pass-secret@api.example/path?api_key=query-secret&x-api-key=header-query-secret", "error": "Authorization=Bearer error-secret password=pass-error https://user:url-secret@host/path?access_token=token-secret"}
	snapshot, _ := AgentEventSnapshot(input)
	for _, secret := range []string{"header-secret", "body-secret", "pass-secret", "query-secret", "header-query-secret", "error-secret", "pass-error", "url-secret", "token-secret"} {
		if strings.Contains(snapshot, secret) {
			t.Errorf("snapshot leaks %q: %s", secret, snapshot)
		}
	}
}

func TestAgentEventSnapshotRedactsHTTPHeadersAndEmbeddedErrorJSON(t *testing.T) {
	input := map[string]any{"headers": map[string]any{"X-API-Key": "header-secret"}, "error": `Invoke: HTTP 400: {"access_token":"error-secret"}`, "huge": `{"password":"large-secret","padding":"` + strings.Repeat("x", 20000) + `"}`}
	snapshot, _ := AgentEventSnapshot(input)
	for _, secret := range []string{"header-secret", "error-secret", "large-secret"} {
		if strings.Contains(snapshot, secret) {
			t.Errorf("snapshot leaks %q", secret)
		}
	}
}

func TestAgentEventSnapshotRetainsTypedNodeOutputMap(t *testing.T) {
	snapshot, _ := AgentEventSnapshot(map[string]map[string]any{"node": {"answer": "done", "api_key": "secret-value"}})
	if !strings.Contains(snapshot, "done") || strings.Contains(snapshot, "secret-value") {
		t.Fatalf("missing or unsafe run results: %s", snapshot)
	}
}

func TestAgentEventSnapshotRetainsToolInvocationAndArtifactLists(t *testing.T) {
	snapshot, _ := AgentEventSnapshot(map[string]any{"tool_invocations": []map[string]any{{"tool_name": "search", "result": "found", "arguments": map[string]any{"api_key": "tool-secret"}}}, "artifacts": []map[string]any{{"name": "report.md"}}})
	if !strings.Contains(snapshot, "search") || !strings.Contains(snapshot, "report.md") || strings.Contains(snapshot, "tool-secret") {
		t.Fatalf("missing or unsafe tool/artifact results: %s", snapshot)
	}
}

func TestAgentEventSnapshotStripsSignedImageURLCredentials(t *testing.T) {
	const signedURL = "https://signed-user:signed-password@storage.googleapis.com/bucket/image.png?GoogleAccessId=service-account-secret&Expires=9999999999&Signature=signed-access-secret#fragment-secret"
	input := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": signedURL}}}}}}
	snapshot, _ := AgentEventSnapshot(input)
	if !strings.Contains(snapshot, "https://storage.googleapis.com/bucket/image.png") {
		t.Fatalf("image location was lost: %s", snapshot)
	}
	for _, secret := range []string{"signed-user", "signed-password", "GoogleAccessId", "service-account-secret", "Expires", "Signature", "signed-access-secret", "fragment-secret"} {
		if strings.Contains(snapshot, secret) {
			t.Errorf("snapshot leaks signed URL component %q: %s", secret, snapshot)
		}
	}
	if got := RedactAgentEventText("fetch failed: " + signedURL); got != "fetch failed: https://storage.googleapis.com/bucket/image.png" {
		t.Fatalf("diagnostic signed URL = %q", got)
	}
}
