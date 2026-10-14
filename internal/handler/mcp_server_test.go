package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/server/config"
)

func TestStandaloneMCPIdentityAndRevocation(t *testing.T) {
	for _, mode := range []string{"self-host", "host"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			revoked := false
			lastKey := ""
			auth := &AuthHandler{userService: &stubUserTokenResolver{getUserByTokenFn: func(ctx context.Context, key string) (*entity.User, common.ErrorCode, error) {
				calls++
				lastKey = key
				if mode == "self-host" && calls == 1 {
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 60*time.Second {
						t.Error("missing bounded key preflight")
					}
				}
				if revoked {
					return nil, common.CodeUnauthorized, errors.New("revoked")
				}
				return &entity.User{ID: key}, common.CodeSuccess, nil
			}}}
			h, err := (&MCPServerHandler{}).NewStandalone(t.Context(), auth, config.MCPConfig{LaunchMode: mode, HostAPIKey: "configured-key", SSE: true, StreamableHTTP: true, JSONResponse: true})
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			warmup := 0
			if mode == "self-host" {
				warmup = 1
			}
			if calls != warmup {
				t.Fatalf("startup auth calls=%d want %d", calls, warmup)
			}
			for _, key := range []string{"alice", "bob"} {
				req := httptest.NewRequest(http.MethodGet, "http://localhost/unknown", nil)
				req.Header.Set("Authorization", "Bearer "+key)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				want := key
				if mode == "self-host" {
					want = "configured-key"
				}
				if w.Code != http.StatusNotFound || lastKey != want {
					t.Fatalf("credential mapping: %d %q", w.Code, lastKey)
				}
			}
			if calls != warmup+2 {
				t.Fatalf("identity cached: %d calls", calls)
			}
			revoked = true
			req := httptest.NewRequest(http.MethodGet, "http://localhost/mcp", nil)
			req.Header.Set("Authorization", "Bearer alice")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("revoked key accepted: %d", w.Code)
			}
		})
	}
}

func TestStandaloneMCPRejectsInvalidConfiguredKey(t *testing.T) {
	auth := &AuthHandler{userService: &stubUserTokenResolver{}}
	h, err := (&MCPServerHandler{}).NewStandalone(t.Context(), auth, config.MCPConfig{LaunchMode: "self-host", HostAPIKey: "do-not-print-me"})
	if h != nil || err == nil || err.Error() != "invalid configured MCP API key" {
		t.Fatalf("invalid preflight result: handler=%v error=%v", h, err)
	}
}
