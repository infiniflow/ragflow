//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package mcpclient is a minimal Model Context Protocol (MCP) client used by
// the Go MCP-management endpoints to list a remote server's tools during
// import and the "test" endpoint. It implements just enough of the spec to
// negotiate a session and call tools/list:
//
//   - streamable-HTTP transport (spec 2025-03-26): single endpoint, JSON-RPC
//     requests via POST, responses either as application/json or as an SSE
//     stream sharing the same connection.
//   - SSE transport (spec 2024-11-05, legacy): server returns an "endpoint"
//     event whose data is the URL the client POSTs JSON-RPC requests to;
//     responses are pushed back on the same SSE stream.
//
// The full Python implementation lives in common/mcp_tool_call_conn.py; this
// is a reduced port focused on tools/list discovery.
package utility

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Transport identifiers. Mirrors common.constants.MCPServerType.
const (
	TransportSSE            = "sse"
	TransportStreamableHTTP = "streamable-http"
)

const (
	protocolVersion = "2025-03-26"
	clientName      = "ragflow-go"
	clientVersion   = "1.0.0"
	jsonRPCVersion  = "2.0"
)

// Tool is the subset of an MCP Tool descriptor returned by tools/list.
// Extra fields surfaced by the server are preserved in Raw so callers can
// round-trip them into variables.tools without losing data.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
	Raw         map[string]interface{} `json:"-"`
}

// FetchOptions controls a single tools/list discovery call.
type FetchOptions struct {
	URL         string
	ServerType  string
	Headers     map[string]string
	Variables   map[string]string
	Timeout     time.Duration
	HTTPClient  *http.Client
	pinHostname string
	pinIP       string
}

// FetchTools opens a connection to the MCP server described by opts and
// returns the tools advertised by tools/list. URL safety / DNS pinning is
// performed here so callers get the same SSRF guarantees the Python path
// has via pin_dns_global + assert_url_is_safe.
func FetchTools(ctx context.Context, opts FetchOptions) ([]Tool, error) {
	if opts.URL == "" {
		return nil, errors.New("Invalid url.")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	hostname, resolvedIP, err := AssertURLSafe(opts.URL)
	if err != nil {
		return nil, err
	}
	opts.pinHostname = hostname
	opts.pinIP = resolvedIP
	if opts.HTTPClient == nil {
		opts.HTTPClient = PinnedHTTPClient(hostname, resolvedIP, opts.Timeout)
	}

	headers, headerErr := renderHeaders(opts.Headers, opts.Variables)
	if headerErr != nil {
		return nil, headerErr
	}

	connectCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	switch strings.ToLower(opts.ServerType) {
	case TransportStreamableHTTP:
		return fetchToolsStreamableHTTP(connectCtx, opts.URL, headers, opts.HTTPClient)
	case TransportSSE:
		return fetchToolsSSE(connectCtx, opts.URL, headers, opts.HTTPClient)
	default:
		return nil, fmt.Errorf("Unsupported MCP server type.")
	}
}

// renderHeaders applies ${name} substitution to header keys and values using
// the supplied variables map, mirroring the Template.safe_substitute pass in
// common/mcp_tool_call_conn.py. Empty keys (after substitution) are dropped.
func renderHeaders(raw map[string]string, vars map[string]string) (map[string]string, error) {
	rendered := map[string]string{}
	for k, v := range raw {
		nk := substituteTemplate(k, vars)
		nv := substituteTemplate(v, vars)
		if strings.TrimSpace(nk) == "" {
			continue
		}
		rendered[nk] = nv
	}
	return rendered, nil
}

// substituteTemplate replaces ${name} occurrences (Python string.Template
// safe-substitute semantics) with values from vars. Unknown keys are left
// in place, matching safe_substitute's behavior.
func substituteTemplate(s string, vars map[string]string) string {
	if vars == nil || !strings.Contains(s, "${") {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		idx := strings.Index(s[i:], "${")
		if idx == -1 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+idx])
		i += idx + 2
		end := strings.Index(s[i:], "}")
		if end == -1 {
			b.WriteString("${")
			b.WriteString(s[i:])
			break
		}
		key := s[i : i+end]
		i += end + 1
		if val, ok := vars[key]; ok {
			b.WriteString(val)
		} else {
			b.WriteString("${")
			b.WriteString(key)
			b.WriteString("}")
		}
	}
	return b.String()
}

// jsonRPCRequest is a JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response. Either Result or Error is set.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func initializeParams() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    clientName,
			"version": clientVersion,
		},
	}
}

// ---------- streamable-HTTP transport ----------

const sessionHeader = "Mcp-Session-Id"

func fetchToolsStreamableHTTP(ctx context.Context, endpoint string, headers map[string]string, client *http.Client) ([]Tool, error) {
	sessionID, initRes, err := streamableSend(ctx, client, endpoint, "", headers, jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		ID:      0,
		Method:  "initialize",
		Params:  initializeParams(),
	}, true)
	if err != nil {
		return nil, err
	}
	if initRes.Error != nil {
		return nil, formatMCPError("initialize", initRes.Error)
	}

	if _, _, err := streamableSend(ctx, client, endpoint, sessionID, headers, jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		Method:  "notifications/initialized",
	}, false); err != nil {
		return nil, err
	}

	_, listRes, err := streamableSend(ctx, client, endpoint, sessionID, headers, jsonRPCRequest{
		JSONRPC: jsonRPCVersion,
		ID:      1,
		Method:  "tools/list",
	}, true)
	if err != nil {
		return nil, err
	}
	if listRes.Error != nil {
		return nil, formatMCPError("tools/list", listRes.Error)
	}
	return parseToolsResult(listRes.Result)
}

// streamableSend POSTs a JSON-RPC payload to the streamable-HTTP endpoint.
// When expectResponse is false (notifications), the response body is not
// parsed. The session id returned by the initial initialize call is
// propagated via the Mcp-Session-Id header per the spec.
func streamableSend(ctx context.Context, client *http.Client, endpoint, sessionID string, headers map[string]string, payload jsonRPCRequest, expectResponse bool) (string, *jsonRPCResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("marshal MCP request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("build MCP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if sessionID != "" {
		req.Header.Set(sessionHeader, sessionID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, mapMCPConnectionError(err)
	}
	defer resp.Body.Close()

	if !expectResponse {
		if resp.StatusCode >= 400 {
			return "", nil, fmt.Errorf("MCP server returned HTTP %d for %s", resp.StatusCode, payload.Method)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return resp.Header.Get(sessionHeader), nil, nil
	}

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", nil, fmt.Errorf("MCP server returned HTTP %d for %s: %s", resp.StatusCode, payload.Method, strings.TrimSpace(string(raw)))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	sid := resp.Header.Get(sessionHeader)
	if sessionID == "" {
		sessionID = sid
	}
	if strings.Contains(contentType, "text/event-stream") {
		r, err := readJSONRPCFromSSE(resp.Body, payload.ID)
		if err != nil {
			return "", nil, err
		}
		return sessionID, r, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", nil, fmt.Errorf("read MCP response: %w", err)
	}
	parsed, err := parseJSONRPC(raw, payload.ID)
	if err != nil {
		return "", nil, err
	}
	return sessionID, parsed, nil
}

// ---------- SSE transport ----------

func fetchToolsSSE(ctx context.Context, endpoint string, headers map[string]string, client *http.Client) ([]Tool, error) {
	streamReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build SSE request: %w", err)
	}
	streamReq.Header.Set("Accept", "text/event-stream")
	streamReq.Header.Set("Cache-Control", "no-cache")
	for k, v := range headers {
		streamReq.Header.Set(k, v)
	}
	streamResp, err := client.Do(streamReq)
	if err != nil {
		return nil, mapMCPConnectionError(err)
	}
	if streamResp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(streamResp.Body, 1<<20))
		streamResp.Body.Close()
		return nil, fmt.Errorf("MCP SSE handshake returned HTTP %d: %s", streamResp.StatusCode, strings.TrimSpace(string(body)))
	}

	stream := newSSEReader(streamResp.Body)
	defer streamResp.Body.Close()

	postURL, err := waitForEndpoint(ctx, stream, endpoint)
	if err != nil {
		return nil, err
	}

	// The endpoint event can hand us an arbitrary absolute URL. A
	// malicious public SSE server could point us at 127.0.0.1 or any
	// other internal host to bounce the POST phase through us. Re-run
	// the SSRF guard against the resolved URL, and — when the host
	// differs from the original SSE host — swap in a fresh pinned
	// client so the dial-time IP override still applies.
	postClient := client
	if postHost, postIP, vErr := AssertURLSafe(postURL); vErr != nil {
		return nil, vErr
	} else if u, perr := url.Parse(postURL); perr == nil && u.Hostname() != "" {
		if u.Hostname() != originalHost(endpoint) {
			postClient = PinnedHTTPClient(postHost, postIP, sseTimeoutFrom(ctx))
		}
	}

	pending := newPendingResponses()
	streamDone := make(chan error, 1)
	go func() {
		streamDone <- stream.dispatch(ctx, pending)
	}()

	postOnce := func(payload jsonRPCRequest) error {
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build SSE POST: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := postClient.Do(req)
		if err != nil {
			return mapMCPConnectionError(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			return fmt.Errorf("MCP server returned HTTP %d for %s: %s", resp.StatusCode, payload.Method, strings.TrimSpace(string(raw)))
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil
	}

	// Register the waiter BEFORE issuing the POST so a fast server that
	// pushes its response before our wait() call doesn't drop the delivery.
	initWaiter := pending.register(0)
	if err := postOnce(jsonRPCRequest{JSONRPC: jsonRPCVersion, ID: 0, Method: "initialize", Params: initializeParams()}); err != nil {
		pending.cancel(0)
		return nil, err
	}
	initRes, err := pending.await(ctx, initWaiter, streamDone)
	if err != nil {
		return nil, err
	}
	if initRes.Error != nil {
		return nil, formatMCPError("initialize", initRes.Error)
	}
	if err := postOnce(jsonRPCRequest{JSONRPC: jsonRPCVersion, Method: "notifications/initialized"}); err != nil {
		return nil, err
	}
	listWaiter := pending.register(1)
	if err := postOnce(jsonRPCRequest{JSONRPC: jsonRPCVersion, ID: 1, Method: "tools/list"}); err != nil {
		pending.cancel(1)
		return nil, err
	}
	listRes, err := pending.await(ctx, listWaiter, streamDone)
	if err != nil {
		return nil, err
	}
	if listRes.Error != nil {
		return nil, formatMCPError("tools/list", listRes.Error)
	}
	return parseToolsResult(listRes.Result)
}

// waitForEndpoint reads SSE events until an "endpoint" event arrives and
// returns the URL to POST JSON-RPC requests to. The data may be either a
// fully-qualified URL or a path; relative paths are resolved against the
// original SSE endpoint.
func waitForEndpoint(ctx context.Context, stream *sseReader, base string) (string, error) {
	for {
		event, err := stream.nextEvent(ctx)
		if err != nil {
			return "", err
		}
		if event == nil {
			return "", errors.New("MCP SSE stream closed before sending endpoint event")
		}
		if event.event == "endpoint" {
			ref := strings.TrimSpace(event.data)
			if ref == "" {
				return "", errors.New("MCP SSE endpoint event has empty data")
			}
			baseURL, err := url.Parse(base)
			if err != nil {
				return "", fmt.Errorf("parse MCP SSE base url: %w", err)
			}
			rel, err := url.Parse(ref)
			if err != nil {
				return "", fmt.Errorf("parse MCP SSE endpoint data: %w", err)
			}
			return baseURL.ResolveReference(rel).String(), nil
		}
		// Other events (heartbeats, message) before endpoint are ignored.
	}
}

// originalHost extracts the hostname from the original SSE endpoint so the
// caller can detect when the server-advertised post URL has moved to a
// different host (and a fresh pinned client is required).
func originalHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// sseTimeoutFrom recovers a non-zero timeout from the request context so
// the freshly-pinned post-phase client has the same deadline as the rest
// of the SSE flow.
func sseTimeoutFrom(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 {
			return d
		}
	}
	return 10 * time.Second
}

// pendingResponses correlates outstanding JSON-RPC ids with channels that
// receive the corresponding response from the SSE dispatcher.
type pendingResponses struct {
	mu      sync.Mutex
	waiters map[string]chan *jsonRPCResponse
}

func newPendingResponses() *pendingResponses {
	return &pendingResponses{waiters: map[string]chan *jsonRPCResponse{}}
}

// pendingWaiter is the handle returned by register; the caller passes it to
// await once the request has been sent.
type pendingWaiter struct {
	key string
	ch  chan *jsonRPCResponse
}

// register reserves a waiter slot for the given JSON-RPC id BEFORE the
// request is sent, so a server that responds before await() is called still
// has somewhere to deliver to.
func (p *pendingResponses) register(id interface{}) pendingWaiter {
	key := normalizeID(id)
	ch := make(chan *jsonRPCResponse, 1)
	p.mu.Lock()
	p.waiters[key] = ch
	p.mu.Unlock()
	return pendingWaiter{key: key, ch: ch}
}

// cancel drops a previously registered waiter. Used when the POST fails so
// a late server delivery cannot block forever in the waiters map.
func (p *pendingResponses) cancel(id interface{}) {
	key := normalizeID(id)
	p.mu.Lock()
	delete(p.waiters, key)
	p.mu.Unlock()
}

// await blocks until the registered waiter's response arrives, the SSE
// stream closes, or ctx expires.
func (p *pendingResponses) await(ctx context.Context, w pendingWaiter, streamDone <-chan error) (*jsonRPCResponse, error) {
	defer func() {
		p.mu.Lock()
		delete(p.waiters, w.key)
		p.mu.Unlock()
	}()
	select {
	case res := <-w.ch:
		return res, nil
	case err := <-streamDone:
		if err == nil {
			return nil, errors.New("MCP SSE stream closed before response arrived")
		}
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *pendingResponses) deliver(res *jsonRPCResponse) {
	key := normalizeID(res.ID)
	p.mu.Lock()
	ch, ok := p.waiters[key]
	p.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- res:
	default:
	}
}

func normalizeID(id interface{}) string {
	switch v := id.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return fmt.Sprintf("%v", v)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// ---------- SSE parsing ----------

type sseEvent struct {
	event string
	data  string
}

type sseReader struct {
	rd *bufio.Reader
}

func newSSEReader(r io.Reader) *sseReader {
	return &sseReader{rd: bufio.NewReaderSize(r, 64*1024)}
}

// nextEvent returns the next SSE event (event: + data:) from the stream, or
// nil when the stream is closed cleanly.
func (s *sseReader) nextEvent(ctx context.Context) (*sseEvent, error) {
	ev := &sseEvent{}
	var dataLines []string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := s.rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if len(dataLines) > 0 || ev.event != "" {
					ev.data = strings.Join(dataLines, "\n")
					return ev, nil
				}
				return nil, nil
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) == 0 && ev.event == "" {
				continue
			}
			ev.data = strings.Join(dataLines, "\n")
			return ev, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if idx := strings.Index(line, ":"); idx >= 0 {
			field := line[:idx]
			value := strings.TrimPrefix(line[idx+1:], " ")
			switch field {
			case "event":
				ev.event = value
			case "data":
				dataLines = append(dataLines, value)
			}
		}
	}
}

// dispatch reads events off the SSE stream and forwards JSON-RPC responses
// to the matching waiter. It returns when the stream closes.
func (s *sseReader) dispatch(ctx context.Context, pending *pendingResponses) error {
	for {
		ev, err := s.nextEvent(ctx)
		if err != nil {
			return err
		}
		if ev == nil {
			return nil
		}
		if ev.event != "" && ev.event != "message" {
			continue
		}
		raw := []byte(ev.data)
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		parsed, err := parseJSONRPC(raw, nil)
		if err != nil {
			continue
		}
		if parsed.Method != "" && parsed.ID == nil {
			// Server-initiated notification; nothing to deliver.
			continue
		}
		pending.deliver(parsed)
	}
}

// readJSONRPCFromSSE consumes a single JSON-RPC response off an inline SSE
// stream returned by a streamable-HTTP POST. The response with matching id
// is returned; everything else is skipped.
func readJSONRPCFromSSE(r io.Reader, wantID interface{}) (*jsonRPCResponse, error) {
	stream := newSSEReader(r)
	for {
		ev, err := stream.nextEvent(context.Background())
		if err != nil {
			return nil, err
		}
		if ev == nil {
			return nil, errors.New("MCP SSE response stream closed before response arrived")
		}
		if ev.event != "" && ev.event != "message" {
			continue
		}
		raw := []byte(ev.data)
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		parsed, err := parseJSONRPC(raw, wantID)
		if err != nil {
			continue
		}
		if normalizeID(parsed.ID) == normalizeID(wantID) {
			return parsed, nil
		}
	}
}

// ---------- shared helpers ----------

func parseJSONRPC(raw []byte, wantID interface{}) (*jsonRPCResponse, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	res := &jsonRPCResponse{}
	if err := dec.Decode(res); err != nil {
		return nil, fmt.Errorf("parse MCP response: %w", err)
	}
	if wantID != nil && res.ID != nil && normalizeID(res.ID) != normalizeID(wantID) {
		return nil, fmt.Errorf("unexpected JSON-RPC id %v (want %v)", res.ID, wantID)
	}
	return res, nil
}

func parseToolsResult(raw json.RawMessage) ([]Tool, error) {
	if len(raw) == 0 {
		return []Tool{}, nil
	}
	var envelope struct {
		Tools []map[string]interface{} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse tools result: %w", err)
	}
	tools := make([]Tool, 0, len(envelope.Tools))
	for _, raw := range envelope.Tools {
		name, _ := raw["name"].(string)
		if name == "" {
			continue
		}
		desc, _ := raw["description"].(string)
		var schema map[string]interface{}
		if s, ok := raw["inputSchema"].(map[string]interface{}); ok {
			schema = s
		}
		tools = append(tools, Tool{
			Name:        name,
			Description: desc,
			InputSchema: schema,
			Raw:         raw,
		})
	}
	return tools, nil
}

func formatMCPError(method string, e *jsonRPCError) error {
	if e == nil {
		return fmt.Errorf("MCP %s failed", method)
	}
	return fmt.Errorf("MCP %s failed (%d): %s", method, e.Code, e.Message)
}

// mapMCPConnectionError surfaces the same wording the Python session uses
// when a low-level connection fails (authentication / network).
func mapMCPConnectionError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("Timeout connecting to MCP server")
	}
	return fmt.Errorf("Connection failed (possibly due to auth error). Please check authentication settings first: %v", err)
}
