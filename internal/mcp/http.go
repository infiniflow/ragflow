package mcp

import (
	"context"
	"crypto/rand"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options selects the Python-equivalent transport and response modes.
type Options struct{ SSE, StreamableHTTP, JSONResponse bool }

type sseSession struct {
	owner     string
	transport *sdk.SSEServerTransport
}

// Handler owns the transport sessions. Close cancels active requests and streams.
type Handler struct {
	handler http.Handler
	cancel  context.CancelFunc
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.handler.ServeHTTP(w, r) }
func (h *Handler) Close() error                                     { h.cancel(); return nil }

// Credential extracts Python-supported credentials without accepting arbitrary
// Authorization schemes. Self-host mode supplies its key in the resolver.
func Credential(headers http.Header) string {
	auth := strings.TrimSpace(headers.Get("Authorization"))
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "Bearer ") {
		if token := strings.TrimSpace(auth[7:]); token != "" {
			return token
		}
	}
	for _, name := range []string{"api_key", "X-API-Key", "Api-Key"} {
		if token := strings.TrimSpace(headers.Get(name)); token != "" {
			return token
		}
	}
	return ""
}

// NewHandler authenticates every request and binds legacy SSE sessions to their
// owner. Wire framing, initialization and dispatch remain SDK responsibilities.
func NewHandler(resolve func(context.Context, string) (string, error), connector func(string) Connector, opts Options) *Handler {
	lifetime, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	sessions := make(map[string]sseSession)
	type identityKey struct{}
	streamable := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return newServer(r.Context(), connector(r.Context().Value(identityKey{}).(string)))
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: opts.JSONResponse, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	if opts.StreamableHTTP {
		mux.Handle("/mcp", streamable)
		mux.Handle("/mcp/", streamable)
	}
	if opts.SSE {
		mux.HandleFunc("GET /sse", func(w http.ResponseWriter, r *http.Request) {
			id := rand.Text()
			transport := &sdk.SSEServerTransport{Endpoint: "/messages/?session_id=" + url.QueryEscape(id), Response: w, MaxRequestBodyBytes: 1 << 20}
			owner := r.Context().Value(identityKey{}).(string)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			// Hold the registry lock through Connect: it publishes the endpoint before
			// returning, so a fast client's first POST must wait until it is ready.
			mu.Lock()
			session, err := newServer(r.Context(), connector(owner)).Connect(r.Context(), transport, nil)
			if err == nil {
				sessions[id] = sseSession{owner, transport}
			}
			mu.Unlock()
			if err != nil {
				return
			}
			defer func() { mu.Lock(); delete(sessions, id); mu.Unlock(); session.Close() }()
			stop := context.AfterFunc(r.Context(), func() { session.Close() })
			defer stop()
			session.Wait()
		})
		mux.HandleFunc("POST /messages/", func(w http.ResponseWriter, r *http.Request) {
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
			id := r.URL.Query().Get("session_id")
			if id == "" {
				id = r.URL.Query().Get("sessionid")
			}
			mu.Lock()
			session, ok := sessions[id]
			mu.Unlock()
			if !ok || session.owner != r.Context().Value(identityKey{}).(string) {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			session.transport.ServeHTTP(w, r)
		})
	}
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Apply the SDK's localhost protection before authentication on BOTH
		// transports; the custom SSE transport does not pass through its handler.
		if local, ok := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr); ok && local.IP.IsLoopback() {
			host := (&url.URL{Host: r.Host}).Hostname()
			if host != "localhost" && !net.ParseIP(host).IsLoopback() {
				http.Error(w, "invalid host", http.StatusForbidden)
				return
			}
		}
		// Reject cross-origin browser access, including GET streams.
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
		}
		ctx, stop := context.WithCancel(r.Context())
		defer stop()
		detach := context.AfterFunc(lifetime, stop)
		defer detach()
		userID, err := resolve(ctx, Credential(r.Header))
		if err != nil || userID == "" {
			http.Error(w, "invalid authorization", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r.WithContext(context.WithValue(ctx, identityKey{}, userID)))
	})
	return &Handler{handler: protected, cancel: cancel}
}
