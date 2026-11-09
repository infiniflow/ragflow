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

package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/graph"
)

// Future AGI tenant export.
//
// A tenant that saved Future AGI keys (tenant_futureagi, see
// FutureAGIService) gets one trace per chat turn: a CHAIN span for the turn,
// a RETRIEVER child when the pipeline retrieved chunks (knowledge base or web
// search), and an LLM child for the answer call. Spans are posted as OTLP/HTTP
// JSON to {endpoint}/tracer/v1/traces with X-Api-Key / X-Secret-Key and the
// resource attribute project_name. Export never blocks or fails a chat: the
// queue is bounded, a full queue drops the trace, and collector errors are
// logged (rate limited) and dropped.

const (
	futureAGICollectorPath   = "/tracer/v1/traces"
	FutureAGIDefaultEndpoint = "https://api.futureagi.com"

	futureAGIQueueSize     = 64
	futureAGIMaxClients    = 128
	futureAGIMaxBodyBytes  = 1 << 20
	futureAGIMaxQueueBytes = 16 << 20
	futureAGIPostTimeout   = 5 * time.Second
	futureAGIMaxDocuments  = 64
	futureAGIMaxDocBytes   = 8 << 10
	futureAGIMaxValueBytes = 64 << 10
	futureAGIMaxErrorBytes = 512
	futureAGIWarnInterval  = time.Minute
	futureAGIRedactedError = "chat failed (error text hidden: redact is on)"
)

// ---------------------------------------------------------------------------
// Client: one per tenant credential set, cached for the process.
// ---------------------------------------------------------------------------

// FutureAGIClient posts OTLP/HTTP traces for one tenant. Writes are async: a
// full queue drops the trace, so the chat path never waits on the collector.
type FutureAGIClient struct {
	Endpoint    string
	APIKey      string
	SecretKey   string
	ProjectName string
	Redact      bool
	HTTP        *http.Client

	workerCtx context.Context
	cancel    context.CancelFunc
	cacheKey  string
	events    chan []byte
	stop      chan struct{}
	stopped   chan struct{}
	once      sync.Once
	dropped   atomic.Int64
	lastWarn  atomic.Int64
}

// NewFutureAGIClient starts a client with its own background worker.
func NewFutureAGIClient(endpoint, apiKey, secretKey, projectName string, redact bool) *FutureAGIClient {
	ctx, cancel := context.WithCancel(context.Background())
	c := &FutureAGIClient{
		workerCtx:   ctx,
		cancel:      cancel,
		Endpoint:    endpoint,
		APIKey:      apiKey,
		SecretKey:   secretKey,
		ProjectName: projectName,
		Redact:      redact,
		HTTP:        &http.Client{Timeout: futureAGIPostTimeout},
		events:      make(chan []byte, futureAGIQueueSize),
		stop:        make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	go c.worker()
	return c
}

// The queue budget is shared across tenants, not multiplied by the client cache.
var futureAGIQueuedBytes atomic.Int64

// enqueue hands one OTLP body to the worker. It never blocks.
func (c *FutureAGIClient) enqueue(body []byte) {
	if c == nil {
		return
	}
	select {
	case <-c.stop:
		c.noteDrop("exporter stopped")
		return
	default:
	}
	if len(body) > futureAGIMaxBodyBytes {
		c.noteDrop("trace size limit")
		return
	}
	for {
		used := futureAGIQueuedBytes.Load()
		if used+int64(len(body)) > futureAGIMaxQueueBytes {
			c.noteDrop("export byte budget full")
			return
		}
		if futureAGIQueuedBytes.CompareAndSwap(used, used+int64(len(body))) {
			break
		}
	}
	select {
	case c.events <- body:
	default:
		futureAGIQueuedBytes.Add(-int64(len(body)))
		c.noteDrop("export queue full")
	}
}

func (c *FutureAGIClient) worker() {
	defer close(c.stopped)
	defer func() {
		if c.cancel != nil {
			c.cancel()
		}
	}()
	for {
		select {
		case <-c.stop:
			// Post what is already queued instead of dropping it.
			for {
				select {
				case body := <-c.events:
					c.send(body)
				default:
					return
				}
			}
		case body := <-c.events:
			c.send(body)
		}
	}
}

func (c *FutureAGIClient) send(body []byte) {
	futureAGIQueuedBytes.Add(-int64(len(body)))
	ctx := c.workerCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	if err := c.post(ctx, body); err != nil {
		c.noteDrop(err.Error())
	}
}

// post sends one OTLP body. The returned error never carries the keys.
func (c *FutureAGIClient) post(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, collectorEndpoint(c.Endpoint), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid Future AGI endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("X-Secret-Key", c.SecretKey)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: futureAGIPostTimeout}
	}
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Future AGI collector unreachable")
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Future AGI collector answered HTTP %d", res.StatusCode)
	}
	return nil
}

// noteDrop counts a dropped trace and logs at most one warning a minute.
func (c *FutureAGIClient) noteDrop(reason string) {
	n := c.dropped.Add(1)
	now := time.Now().UnixNano()
	last := c.lastWarn.Load()
	if last != 0 && time.Duration(now-last) < futureAGIWarnInterval {
		return
	}
	if !c.lastWarn.CompareAndSwap(last, now) {
		return
	}
	common.Warn("Future AGI: chat trace dropped; chats are not affected",
		zap.String("reason", reason),
		zap.String("project_name", c.ProjectName),
		zap.Int64("dropped_total", n))
}

// Shutdown posts queued traces and stops the worker, bounded by ctx.
func (c *FutureAGIClient) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.once.Do(func() { close(c.stop) })
	select {
	case <-c.stopped:
		return nil
	case <-ctx.Done():
		if c.cancel != nil {
			c.cancel()
		}
		return ctx.Err()
	}
}

func collectorEndpoint(endpoint string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if trimmed == "" {
		trimmed = FutureAGIDefaultEndpoint
	}
	if strings.HasSuffix(trimmed, futureAGICollectorPath) {
		return trimmed
	}
	return trimmed + futureAGICollectorPath
}

var futureAGIClients = struct {
	sync.Mutex
	byTenant map[string]*FutureAGIClient
}{byTenant: map[string]*FutureAGIClient{}}

func futureAGIRowKey(row *entity.TenantFutureAGI) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		row.TenantID, row.Endpoint, row.APIKey, row.SecretKey, row.ProjectName, fmt.Sprint(row.Redact),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// futureAGIClientForTenant returns the cached client for the tenant's current
// settings row, or nil when the tenant has no complete row. A lookup failure
// is non-fatal: tracing is not a chat requirement.
func futureAGIClientForTenant(ctx context.Context, tenantID string) *FutureAGIClient {
	if tenantID == "" || dao.DB == nil {
		return nil
	}
	var rows []entity.TenantFutureAGI
	if err := dao.DB.WithContext(ctx).Where("tenant_id = ?", tenantID).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil
	}
	row := rows[0]
	if strings.TrimSpace(row.APIKey) == "" || strings.TrimSpace(row.SecretKey) == "" || strings.TrimSpace(row.ProjectName) == "" {
		return nil
	}
	if strings.TrimSpace(row.Endpoint) == "" {
		row.Endpoint = FutureAGIDefaultEndpoint
	}
	key := futureAGIRowKey(&row)

	futureAGIClients.Lock()
	defer futureAGIClients.Unlock()
	if c, ok := futureAGIClients.byTenant[tenantID]; ok {
		if c.cacheKey == key {
			return c
		}
		retireFutureAGIClient(c)
	}
	if len(futureAGIClients.byTenant) >= futureAGIMaxClients {
		for id, old := range futureAGIClients.byTenant {
			delete(futureAGIClients.byTenant, id)
			retireFutureAGIClient(old)
			break
		}
	}
	c := NewFutureAGIClient(row.Endpoint, row.APIKey, row.SecretKey, row.ProjectName, row.Redact)
	c.cacheKey = key
	futureAGIClients.byTenant[tenantID] = c
	return c
}

// forgetFutureAGIClient retires the tenant's cached client after its settings
// changed or were deleted. Queued traces are still posted.
func forgetFutureAGIClient(tenantID string) {
	futureAGIClients.Lock()
	defer futureAGIClients.Unlock()
	if c, ok := futureAGIClients.byTenant[tenantID]; ok {
		delete(futureAGIClients.byTenant, tenantID)
		retireFutureAGIClient(c)
	}
}

func retireFutureAGIClient(c *FutureAGIClient) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*futureAGIPostTimeout)
		defer cancel()
		_ = c.Shutdown(ctx)
	}()
}

// futureAGIInflight counts turns that started and have not finished yet, so
// shutdown can wait for a turn whose pipeline is still closing its channel.
var futureAGIInflight sync.WaitGroup

// ShutdownFutureAGIClients waits for in-flight turns, then posts queued traces
// of every tenant, all bounded by ctx. The API server calls it after the HTTP
// server stopped.
func ShutdownFutureAGIClients(ctx context.Context) {
	idle := make(chan struct{})
	go func() {
		futureAGIInflight.Wait()
		close(idle)
	}()
	select {
	case <-idle:
	case <-ctx.Done():
	}
	futureAGIClients.Lock()
	clients := make([]*FutureAGIClient, 0, len(futureAGIClients.byTenant))
	for tenantID, c := range futureAGIClients.byTenant {
		clients = append(clients, c)
		delete(futureAGIClients.byTenant, tenantID)
	}
	futureAGIClients.Unlock()
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(c *FutureAGIClient) {
			defer wg.Done()
			_ = c.Shutdown(ctx)
		}(c)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Turn: the spans of one chat turn, built while the pipeline runs.
// ---------------------------------------------------------------------------

type futureAGITurnKeyType struct{}

var futureAGITurnKey = futureAGITurnKeyType{}

type futureAGISpanData struct {
	spanID   string
	parentID string
	name     string
	start    time.Time
	end      time.Time
	attrs    []map[string]any
	errText  string
}

type futureAGILLMCall struct {
	spanID   string
	start    time.Time
	model    string
	provider string
	messages []modelModule.Message
}

type futureAGITurn struct {
	client    *FutureAGIClient
	traceID   string
	rootID    string
	start     time.Time
	chatID    string
	sessionID string
	tenantID  string
	question  string
	stream    bool
	tracked   bool

	mu       sync.Mutex
	pipeline string
	children []futureAGISpanData
	llm      *futureAGILLMCall
	done     bool
}

// AsyncChat runs the chat pipeline and, when the chat's tenant has Future AGI
// keys, exports the turn. Every caller (chat sessions, the OpenAI-compatible
// endpoint, embedded chatbots) goes through here.
func (s *ChatPipelineService) AsyncChat(
	ctx context.Context,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
	kwargs map[string]interface{},
) (<-chan AsyncChatResult, error) {
	ctx, turn := startFutureAGITurn(ctx, chat, messages, stream)
	results, err := s.asyncChat(ctx, chat, messages, stream, kwargs)
	return turn.observe(ctx, results, err)
}

func startFutureAGITurn(ctx context.Context, chat *entity.Chat, messages []map[string]interface{}, stream bool) (context.Context, *futureAGITurn) {
	if chat == nil {
		return ctx, nil
	}
	client := futureAGIClientForTenant(ctx, chat.TenantID)
	if client == nil {
		return ctx, nil
	}
	sessionID := common.SessionIDFromContext(ctx)
	if sessionID == "" {
		sessionID = chat.ID
	}
	turn := &futureAGITurn{
		client:    client,
		traceID:   newFutureAGITraceID(),
		rootID:    newFutureAGISpanID(),
		start:     time.Now(),
		chatID:    chat.ID,
		sessionID: sessionID,
		tenantID:  chat.TenantID,
		question:  lastUserQuestion(messages),
		stream:    stream,
		tracked:   true,
	}
	futureAGIInflight.Add(1)
	return context.WithValue(ctx, futureAGITurnKey, turn), turn
}

func futureAGITurnFrom(ctx context.Context) *futureAGITurn {
	if ctx == nil {
		return nil
	}
	turn, _ := ctx.Value(futureAGITurnKey).(*futureAGITurn)
	return turn
}

// observe forwards the pipeline's results unchanged and finishes the turn
// when the pipeline closes its channel.
func (t *futureAGITurn) observe(ctx context.Context, in <-chan AsyncChatResult, err error) (<-chan AsyncChatResult, error) {
	if t == nil {
		return in, err
	}
	if err != nil || in == nil {
		// A request the pipeline rejects up front (empty messages, last
		// message not from the user) is not a chat turn: nothing is exported.
		t.abandon()
		return in, err
	}
	out := make(chan AsyncChatResult, cap(in))
	go func() {
		defer close(out)
		var streamed strings.Builder
		var final *AsyncChatResult
		forwarding := true
		for r := range in {
			if r.Final {
				rr := r
				final = &rr
			} else if r.Answer != "" && streamed.Len() < futureAGIMaxValueBytes {
				streamed.WriteString(futureAGIValue(r.Answer, futureAGIMaxValueBytes-streamed.Len()))
			}
			if !forwarding {
				continue
			}
			select {
			case out <- r:
			case <-ctx.Done():
				// The consumer is gone. Keep draining so the pipeline
				// goroutine can finish, but stop forwarding.
				forwarding = false
			}
		}
		answer := streamed.String()
		if final != nil && final.Answer != "" {
			answer = final.Answer
		}
		errText := ""
		if strings.Contains(answer, "**ERROR**") {
			errText = answer
		}
		t.finish(answer, errText, final != nil)
	}()
	return out, nil
}

// retrieved records the retrieval phase. ran is false when the dialog's prompt
// has no {knowledge} parameter, in which case no retrieval happened.
func (t *futureAGITurn) retrieved(query string, kbinfos map[string]interface{}, ran bool, took time.Duration, chat *entity.Chat) {
	if t == nil || !ran {
		return
	}
	end := time.Now()
	chunks := futureAGIChunks(kbinfos)
	redact := t.client.Redact
	attrs := []map[string]any{
		stringAttr("fi.span.kind", "RETRIEVER"),
		stringAttr("session.id", t.sessionID),
		intAttr("ragflow.retrieval.chunk_count", len(chunks)),
	}
	if !redact && query != "" {
		attrs = append(attrs, stringAttr("input.value", futureAGIValue(query, futureAGIMaxValueBytes)))
	}
	if chat != nil {
		attrs = append(attrs,
			intAttr("ragflow.kb_count", len(kbIDStringsFromChat(chat))),
			intAttr("ragflow.retrieval.top_n", int(chat.TopN)),
			intAttr("ragflow.retrieval.top_k", int(chat.TopK)),
			doubleAttr("ragflow.retrieval.similarity_threshold", chat.SimilarityThreshold),
			doubleAttr("ragflow.retrieval.vector_similarity_weight", chat.VectorSimilarityWeight),
		)
		if chat.RerankID != "" {
			attrs = append(attrs, stringAttr("ragflow.retrieval.rerank_model", chat.RerankID))
		}
	}
	var summary []map[string]any
	for i, ck := range chunks {
		if i >= futureAGIMaxDocuments {
			attrs = append(attrs, boolAttr("ragflow.retrieval.documents_truncated", true))
			break
		}
		prefix := fmt.Sprintf("retrieval.documents.%d.document.", i)
		id := firstMapString(ck, "chunk_id", "id")
		docID := firstMapString(ck, "doc_id")
		if redact && firstMapString(ck, "url") != "" {
			// Web-search chunk ids are built from the page URL; with redact
			// on, export a stable hash instead of the URL.
			id, docID = futureAGIHashID("web", id), ""
		}
		if id != "" {
			attrs = append(attrs, stringAttr(prefix+"id", id))
		}
		meta := map[string]any{}
		if docID != "" {
			meta["doc_id"] = docID
		}
		if v := firstMapString(ck, "kb_id"); v != "" {
			meta["kb_id"] = v
		}
		entry := map[string]any{"id": id}
		if score, ok := mapFloat(ck, "similarity"); ok {
			attrs = append(attrs, doubleAttr(prefix+"score", score))
			entry["score"] = score
		}
		for _, k := range []string{"vector_similarity", "term_similarity"} {
			if v, ok := mapFloat(ck, k); ok {
				meta[k] = v
			}
		}
		if !redact {
			if v := firstMapString(ck, "docnm_kwd", "document_name"); v != "" {
				meta["document_name"] = v
				entry["document_name"] = v
			}
			if v := firstMapString(ck, "url"); v != "" {
				meta["url"] = v
			}
			if text := firstMapString(ck, "content_with_weight", "content"); text != "" {
				attrs = append(attrs, stringAttr(prefix+"content", futureAGIValue(text, futureAGIMaxDocBytes)))
			}
		}
		if len(meta) > 0 {
			if b, err := json.Marshal(meta); err == nil {
				attrs = append(attrs, stringAttr(prefix+"metadata", string(b)))
			}
		}
		summary = append(summary, entry)
	}
	if !redact {
		if b, err := json.Marshal(summary); err == nil {
			attrs = append(attrs, stringAttr("output.value", futureAGIValue(string(b), futureAGIMaxValueBytes)))
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return
	}
	t.pipeline = "rag"
	t.children = append(t.children, futureAGISpanData{
		spanID:   newFutureAGISpanID(),
		parentID: t.rootID,
		name:     "ragflow.retrieval",
		start:    end.Add(-took),
		end:      end,
		attrs:    attrs,
	})
}

// llmStart records the start of the answer call with the messages sent.
func (t *futureAGITurn) llmStart(model, provider string, messages []modelModule.Message) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return
	}
	if t.pipeline == "" {
		t.pipeline = "solo"
	}
	t.llm = &futureAGILLMCall{
		spanID:   newFutureAGISpanID(),
		start:    time.Now(),
		model:    model,
		provider: provider,
		messages: messages,
	}
}

// llmEnd closes the answer call. promptTokens and completionTokens are the
// counts RAGFlow itself computes for the turn (cl100k), the same numbers it
// reports to Langfuse.
func (t *futureAGITurn) llmEnd(output string, promptTokens, completionTokens int, callErr error) {
	if t == nil {
		return
	}
	errText := ""
	if callErr != nil {
		errText = callErr.Error()
		if errText == "" {
			errText = "chat model call failed"
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeLLMLocked(output, promptTokens, completionTokens, errText)
}

func (t *futureAGITurn) closeLLMLocked(output string, promptTokens, completionTokens int, errText string) {
	if t.done || t.llm == nil {
		return
	}
	call := t.llm
	t.llm = nil
	redact := t.client.Redact
	attrs := []map[string]any{
		stringAttr("fi.span.kind", "LLM"),
		stringAttr("gen_ai.operation.name", "chat"),
		stringAttr("session.id", t.sessionID),
	}
	if call.model != "" {
		attrs = append(attrs,
			stringAttr("llm.model_name", call.model),
			stringAttr("gen_ai.request.model", call.model),
		)
	}
	if call.provider != "" {
		attrs = append(attrs, stringAttr("gen_ai.provider.name", call.provider))
	}
	if !redact {
		rendered := make([]map[string]any, 0, len(call.messages))
		for i, m := range call.messages {
			if i >= 64 {
				break
			}
			content := futureAGIValue(futureAGIMessageContent(m.Content), futureAGIMaxValueBytes)
			prefix := fmt.Sprintf("llm.input_messages.%d.message.", i)
			attrs = append(attrs,
				stringAttr(prefix+"role", m.Role),
				stringAttr(prefix+"content", futureAGIValue(content, futureAGIMaxValueBytes)),
			)
			rendered = append(rendered, map[string]any{"role": m.Role, "content": content})
		}
		if b, err := json.Marshal(rendered); err == nil {
			attrs = append(attrs, stringAttr("input.value", futureAGIValue(string(b), futureAGIMaxValueBytes)))
		}
		if output != "" && errText == "" {
			attrs = append(attrs,
				stringAttr("output.value", futureAGIValue(output, futureAGIMaxValueBytes)),
				stringAttr("llm.output_messages.0.message.role", "assistant"),
				stringAttr("llm.output_messages.0.message.content", futureAGIValue(output, futureAGIMaxValueBytes)),
			)
		}
	}
	if promptTokens > 0 || completionTokens > 0 {
		attrs = append(attrs,
			intAttr("llm.token_count.prompt", promptTokens),
			intAttr("llm.token_count.completion", completionTokens),
			intAttr("llm.token_count.total", promptTokens+completionTokens),
			intAttr("gen_ai.usage.input_tokens", promptTokens),
			intAttr("gen_ai.usage.output_tokens", completionTokens),
			stringAttr("ragflow.usage.source", "ragflow_cl100k_count"),
		)
	}
	t.children = append(t.children, futureAGISpanData{
		spanID:   call.spanID,
		parentID: t.rootID,
		name:     "ragflow.llm",
		start:    call.start,
		end:      time.Now(),
		attrs:    attrs,
		errText:  errText,
	})
}

// abandon ends a turn without exporting it.
func (t *futureAGITurn) abandon() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.done {
		t.done = true
		t.release()
	}
}

func (t *futureAGITurn) release() {
	if t.tracked {
		t.tracked = false
		futureAGIInflight.Done()
	}
}

// finish closes the turn and queues its spans. complete is false when the
// pipeline closed without a final result (the client went away).
func (t *futureAGITurn) finish(answer, errText string, complete bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.done {
		t.mu.Unlock()
		return
	}
	if t.llm != nil {
		// Paths that do not report their own end (the LLM-only chat) are
		// closed with the turn's answer and RAGFlow's own token count.
		prompt, completion := 0, 0
		if errText == "" && answer != "" {
			prompt = futureAGICountTokens(futureAGIMessagesText(t.llm.messages))
			completion = futureAGICountTokens(answer)
		}
		out := answer
		if errText != "" {
			out = ""
		}
		t.closeLLMLocked(out, prompt, completion, errText)
	}
	t.done = true
	children := t.children
	pipeline := t.pipeline
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.release()
		t.mu.Unlock()
	}()

	if pipeline == "" {
		pipeline = "other"
	}
	redact := t.client.Redact
	attrs := []map[string]any{
		stringAttr("fi.span.kind", "CHAIN"),
		stringAttr("session.id", t.sessionID),
		stringAttr("gen_ai.conversation.id", t.sessionID),
		stringAttr("user.id", t.tenantID),
		stringAttr("ragflow.tenant_id", t.tenantID),
		stringAttr("ragflow.chat_id", t.chatID),
		stringAttr("ragflow.pipeline", pipeline),
		boolAttr("ragflow.stream", t.stream),
	}
	if !complete {
		attrs = append(attrs, boolAttr("ragflow.incomplete", true))
	}
	if redact {
		attrs = append(attrs, boolAttr("ragflow.redacted", true))
	} else {
		if t.question != "" {
			attrs = append(attrs, stringAttr("input.value", futureAGIValue(t.question, futureAGIMaxValueBytes)))
		}
		if answer != "" {
			attrs = append(attrs, stringAttr("output.value", futureAGIValue(futureAGIErrorOrValue(answer, errText), futureAGIMaxValueBytes)))
		}
	}
	root := futureAGISpanData{
		spanID:  t.rootID,
		name:    "ragflow.chat",
		start:   t.start,
		end:     time.Now(),
		attrs:   attrs,
		errText: errText,
	}
	spans := append([]futureAGISpanData{root}, children...)
	body, err := json.Marshal(t.client.payload(t.traceID, spans))
	if err != nil {
		t.client.noteDrop("encode failed")
		return
	}
	t.client.enqueue(body)
}

// payload renders spans as one OTLP/JSON export request.
func (c *FutureAGIClient) payload(traceID string, spans []futureAGISpanData) map[string]any {
	rendered := make([]any, 0, len(spans))
	for _, s := range spans {
		status := map[string]any{"code": 1}
		if s.errText != "" {
			status = map[string]any{"code": 2, "message": futureAGIErrorText(s.errText, c.Redact)}
		}
		span := map[string]any{
			"traceId":           traceID,
			"spanId":            s.spanID,
			"name":              s.name,
			"kind":              1,
			"startTimeUnixNano": fmt.Sprintf("%d", s.start.UnixNano()),
			"endTimeUnixNano":   fmt.Sprintf("%d", s.end.UnixNano()),
			"attributes":        s.attrs,
			"status":            status,
		}
		if s.parentID != "" {
			span["parentSpanId"] = s.parentID
		}
		rendered = append(rendered, span)
	}
	return map[string]any{
		"resourceSpans": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []map[string]any{
						stringAttr("project_name", c.ProjectName),
						stringAttr("project_type", "observe"),
						stringAttr("service.name", "ragflow"),
					},
				},
				"scopeSpans": []any{
					map[string]any{
						"scope": map[string]any{"name": "ragflow-futureagi"},
						"spans": rendered,
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Value helpers and scrubbing.
// ---------------------------------------------------------------------------

var (
	futureAGIURLPattern   = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s"'<>\x60]+`)
	futureAGIEmailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	futureAGIIPPattern    = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?\b`)
	futureAGIPathPattern  = regexp.MustCompile(`(?:[A-Za-z]:\\|~/|/)(?:[\w.\-]+[/\\]){1,}[\w.\-]*`)
	futureAGIDataURI      = regexp.MustCompile(`data:[\w/+.\-]{1,64};base64,[A-Za-z0-9+/=]+`)

	futureAGIEnvOnce sync.Once
	futureAGIHome    string
	futureAGIUser    string
	futureAGIHost    string
)

func futureAGIEnv() (home, username, host string) {
	futureAGIEnvOnce.Do(func() {
		if h, err := os.UserHomeDir(); err == nil && len(h) > 1 {
			futureAGIHome = strings.TrimRight(h, "/")
		}
		if u, err := user.Current(); err == nil && len(u.Username) >= 3 {
			futureAGIUser = u.Username
		}
		if h, err := os.Hostname(); err == nil && len(h) >= 3 {
			futureAGIHost = h
		}
	})
	return futureAGIHome, futureAGIUser, futureAGIHost
}

// futureAGIValue caps an exported string and masks the server's home
// directory, which can show up in attachment or tool text.
func futureAGIValue(s string, limit int) string {
	if home, _, _ := futureAGIEnv(); home != "" {
		s = strings.ReplaceAll(s, home, "~")
	}
	if len(s) > limit {
		cut := limit
		for cut > 0 && !utf8RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…[truncated]"
	}
	return s
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

// futureAGIErrorText turns raw error text into a status message: with redact
// on it is a fixed line; otherwise URLs, IPs, e-mail addresses, file paths and
// the server's user and host names are masked and the text is capped.
func futureAGIErrorText(raw string, redact bool) string {
	if redact {
		return futureAGIRedactedError
	}
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "**ERROR**:"))
	if s == "" {
		s = "chat failed"
	}
	return futureAGIScrubError(s)
}

func futureAGIScrubError(s string) string {
	home, username, host := futureAGIEnv()
	if home != "" {
		s = strings.ReplaceAll(s, home, "~")
	}
	s = futureAGIURLPattern.ReplaceAllString(s, "<url>")
	s = futureAGIEmailPattern.ReplaceAllString(s, "<email>")
	s = futureAGIIPPattern.ReplaceAllString(s, "<ip>")
	s = futureAGIPathPattern.ReplaceAllString(s, "<path>")
	if host != "" {
		s = strings.ReplaceAll(s, host, "<host>")
	}
	if username != "" {
		s = regexp.MustCompile(`\b`+regexp.QuoteMeta(username)+`\b`).ReplaceAllString(s, "<user>")
	}
	if len(s) > futureAGIMaxErrorBytes {
		s = futureAGIValue(s, futureAGIMaxErrorBytes)
	}
	return s
}

// futureAGIErrorOrValue scrubs the turn output when it is an error line.
func futureAGIErrorOrValue(answer, errText string) string {
	if errText == "" {
		return answer
	}
	return "**ERROR**: " + futureAGIErrorText(errText, false)
}

func futureAGIMessageContent(content interface{}) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return futureAGIDataURI.ReplaceAllString(string(b), "[image]")
	}
}

func futureAGIMessagesText(messages []modelModule.Message) string {
	var b strings.Builder
	for _, m := range messages {
		b.WriteString(futureAGIMessageContent(m.Content))
		b.WriteString("\n")
	}
	return b.String()
}

// futureAGICountTokens uses RAGFlow's own cl100k counter. It never panics into
// the chat path (the counter panics when the BPE table is missing).
func futureAGICountTokens(s string) (n int) {
	defer func() {
		if recover() != nil {
			n = 0
		}
	}()
	if s == "" {
		return 0
	}
	return graph.NumTokensFromString(s)
}

func futureAGIChunks(kbinfos map[string]interface{}) []map[string]interface{} {
	if kbinfos == nil {
		return nil
	}
	switch v := kbinfos["chunks"].(type) {
	case []map[string]interface{}:
		return v
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(v))
		for _, raw := range v {
			if m, ok := raw.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func kbIDStringsFromChat(chat *entity.Chat) []string {
	ids := make([]string, 0, len(chat.KBIDs))
	for _, raw := range chat.KBIDs {
		if id, ok := raw.(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func firstMapString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func mapFloat(m map[string]interface{}, key string) (float64, bool) {
	switch v := m[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	return 0, false
}

// llmModelName is the configured chat model name, or "" when it is unset.
func llmModelName(llmModelConfig map[string]interface{}) string {
	if name, ok := llmModelConfig["llm_name"].(string); ok {
		return name
	}
	return ""
}

// llmFactoryName is the configured chat model provider, or "".
func llmFactoryName(llmModelConfig map[string]interface{}) string {
	if name, ok := llmModelConfig["llm_factory"].(string); ok {
		return name
	}
	return ""
}

func stringAttr(key, value string) map[string]any {
	return map[string]any{"key": key, "value": map[string]any{"stringValue": value}}
}

func intAttr(key string, value int) map[string]any {
	return map[string]any{"key": key, "value": map[string]any{"intValue": fmt.Sprintf("%d", value)}}
}

func doubleAttr(key string, value float64) map[string]any {
	return map[string]any{"key": key, "value": map[string]any{"doubleValue": value}}
}

func boolAttr(key string, value bool) map[string]any {
	return map[string]any{"key": key, "value": map[string]any{"boolValue": value}}
}

func futureAGIHashID(prefix, value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return prefix + "-" + hex.EncodeToString(sum[:8])
}

func newFutureAGITraceID() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return hex.EncodeToString(id[:])
}

func newFutureAGISpanID() string {
	var id [8]byte
	_, _ = rand.Read(id[:])
	return hex.EncodeToString(id[:])
}
