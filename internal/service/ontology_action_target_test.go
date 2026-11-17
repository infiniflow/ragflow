package service

import (
	"context"
	"strings"
	"testing"

	mcpclient "ragflow/internal/mcp/client"
)

// A binding may carry the address itself. That path never touches the database,
// which is what lets a run test reach an endpoint before it is registered, so it
// must not depend on a tenant row existing.
func TestResolveMCPTargetAcceptsAddress(t *testing.T) {
	target, err := resolveMCPTarget(context.Background(), nil, "tenant-1", "http://127.0.0.1:18080/mcp")
	if err != nil {
		t.Fatalf("resolveMCPTarget: %v", err)
	}
	if target.URL != "http://127.0.0.1:18080/mcp" {
		t.Fatalf("URL = %q, want the address itself", target.URL)
	}
	if target.ServerType != mcpclient.TransportStreamableHTTP {
		t.Fatalf("ServerType = %q, want %q", target.ServerType, mcpclient.TransportStreamableHTTP)
	}
	if target.Registered {
		t.Fatal("an address typed into the binding is not a registered server")
	}
	if len(target.Headers) != 0 || len(target.Variables) != 0 {
		t.Fatalf("ad-hoc address must carry no headers/variables, got %v/%v", target.Headers, target.Variables)
	}
}

// The legacy SSE endpoint is recognised by its conventional path, because an
// address with no server type has no other signal to go on.
func TestResolveMCPTargetUsesSSEForSSEPath(t *testing.T) {
	for _, address := range []string{
		"https://example.com/sse",
		"https://example.com/sse/",
		"HTTPS://EXAMPLE.COM/sse",
	} {
		target, err := resolveMCPTarget(context.Background(), nil, "tenant-1", address)
		if err != nil {
			t.Fatalf("resolveMCPTarget(%q): %v", address, err)
		}
		if target.ServerType != mcpclient.TransportSSE {
			t.Fatalf("resolveMCPTarget(%q).ServerType = %q, want sse", address, target.ServerType)
		}
	}
	// A path that merely contains "sse" is not the SSE endpoint.
	target, err := resolveMCPTarget(context.Background(), nil, "tenant-1", "https://example.com/mcp?x=sse")
	if err != nil {
		t.Fatalf("resolveMCPTarget: %v", err)
	}
	if target.ServerType != mcpclient.TransportStreamableHTTP {
		t.Fatalf("ServerType = %q, want streamable-http", target.ServerType)
	}
}

func TestResolveMCPTargetRejectsEmpty(t *testing.T) {
	_, err := resolveMCPTarget(context.Background(), nil, "tenant-1", "   ")
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected an empty-server error, got %v", err)
	}
}
