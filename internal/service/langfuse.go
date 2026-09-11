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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

type langfuseCtxKeyType struct{}

var langfuseCtxKey = langfuseCtxKeyType{}

// LangfuseClientFromTenant returns a tracing client for the given tenant,
// or nil if Langfuse is not configured. Failures to look up credentials
// are non-fatal; Langfuse is observability, not a chat path requirement.
func LangfuseClientFromTenant(ctx context.Context, tenantID, userID, chatID, modelName string) *LangfuseClient {
	if tenantID == "" {
		return nil
	}
	creds, err := getTenantLangfuse(tenantID)
	if err != nil || creds == nil {
		return nil
	}
	if creds.Host == "" || creds.PublicKey == "" || creds.SecretKey == "" {
		return nil
	}
	return NewLangfuseClient(creds.Host, creds.PublicKey, creds.SecretKey)
}

// getTenantLangfuse returns the Langfuse credentials for a tenant, or
// (nil, nil) when no row exists.
func getTenantLangfuse(tenantID string) (*entity.TenantLangfuse, error) {
	if tenantID == "" {
		return nil, gorm.ErrInvalidDB
	}
	var row entity.TenantLangfuse
	err := dao.DB.Where("tenant_id = ?", tenantID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// LangfuseClient posts trace and observation events to a Langfuse ingestion
// endpoint. All writes are async (background worker drains a buffered
// channel); reads (none in this minimal version) are direct.
type LangfuseClient struct {
	Host      string
	PublicKey string
	SecretKey string
	HTTP      *http.Client

	events  chan []byte
	stop    chan struct{}
	stopped chan struct{}
	once    sync.Once
}

// NewLangfuseClient constructs a LangfuseClient with a 2-second HTTP timeout
// and starts a background worker. Call Shutdown to drain pending events.
func NewLangfuseClient(host, publicKey, secretKey string) *LangfuseClient {
	c := &LangfuseClient{
		Host:      host,
		PublicKey: publicKey,
		SecretKey: secretKey,
		HTTP:      &http.Client{Timeout: 2 * time.Second},
		events:    make(chan []byte, 1024),
		stop:      make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	go c.worker()
	return c
}

// LangfuseTrace is a single Langfuse trace (one per request).
type LangfuseTrace struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	UserID    string                 `json:"userId,omitempty"`
	SessionID string                 `json:"sessionId,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp string                 `json:"timestamp"`
}

// LangfuseSpan is a unit of work within a trace (e.g. "Pre-retrieval processing").
type LangfuseSpan struct {
	ID                  string                 `json:"id"`
	TraceID             string                 `json:"traceId"`
	ParentObservationID string                 `json:"parentObservationId,omitempty"`
	Name                string                 `json:"name"`
	StartTime           string                 `json:"startTime"`
	EndTime             string                 `json:"endTime,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	Input               interface{}            `json:"input,omitempty"`
	Output              interface{}            `json:"output,omitempty"`
}

// LangfuseGeneration is a span with model, usage, and LLM-specific fields.
type LangfuseGeneration struct {
	ID                  string                 `json:"id"`
	TraceID             string                 `json:"traceId"`
	ParentObservationID string                 `json:"parentObservationId,omitempty"`
	Name                string                 `json:"name"`
	Model               string                 `json:"model,omitempty"`
	StartTime           string                 `json:"startTime"`
	EndTime             string                 `json:"endTime,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	Input               interface{}            `json:"input,omitempty"`
	Output              interface{}            `json:"output,omitempty"`
	Usage               *LangfuseUsage         `json:"usage,omitempty"`
}

// LangfuseUsage records prompt/completion/total token counts.
type LangfuseUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

func (c *LangfuseClient) PostTrace(ctx context.Context, t LangfuseTrace) error {
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return c.enqueue("traces", body)
}

func (c *LangfuseClient) PostSpan(ctx context.Context, s LangfuseSpan) error {
	body, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return c.enqueue("observations", body)
}

func (c *LangfuseClient) PostGeneration(ctx context.Context, g LangfuseGeneration) error {
	body, err := json.Marshal(g)
	if err != nil {
		return err
	}
	return c.enqueue("observations", body)
}

func (c *LangfuseClient) enqueue(kind string, body []byte) error {
	if c == nil {
		return fmt.Errorf("nil langfuse client")
	}
	envelope := struct {
		Kind string `json:"kind"`
		Body []byte `json:"body"`
	}{Kind: kind, Body: body}
	env, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	select {
	case c.events <- env:
		return nil
	default:
		return nil
	}
}

func (c *LangfuseClient) worker() {
	defer close(c.stopped)
	for {
		select {
		case <-c.stop:
			drainCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			for {
				select {
				case ev := <-c.events:
					c.post(drainCtx, ev)
				case <-drainCtx.Done():
					cancel()
					return
				default:
					cancel()
					return
				}
			}
		case ev := <-c.events:
			c.post(context.Background(), ev)
		}
	}
}

func (c *LangfuseClient) post(ctx context.Context, envelope []byte) {
	var env struct {
		Kind string          `json:"kind"`
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(envelope, &env); err != nil {
		return
	}
	url := c.Host + "/api/public/" + env.Kind
	auth := basicAuth(c.PublicKey, c.SecretKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(env.Body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	// per tenant by an operator (see entity.TenantLangfuse), not by
	// the requesting user. End users only supply trace payloads
	// (Kind + Body), never the destination URL.
	// codeql[go/request-forgery] False positive: c.Host is configured
	res, err := c.HTTP.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
}

func (c *LangfuseClient) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.once.Do(func() { close(c.stop) })
	select {
	case <-c.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func basicAuth(public, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(public+":"+secret))
}

// ErrLangfuseUnauthorized indicates the Langfuse credentials were rejected
var ErrLangfuseUnauthorized = errors.New("langfuse: unauthorized")

type LangfuseAPIError struct {
	StatusCode int
	Body       string
}

func (e *LangfuseAPIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("langfuse: unexpected status %d", e.StatusCode)
	}
	return fmt.Sprintf("langfuse: unexpected status %d: %s", e.StatusCode, e.Body)
}

func IsLangfuseAPIError(err error) bool {
	var apiErr *LangfuseAPIError
	return errors.As(err, &apiErr)
}

// langfuseProjectsResponse mirrors the body of GET /api/public/projects.
type langfuseProjectsResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

// GetProject calls GET {host}/api/public/projects and returns the first
// project's id and name.
func (c *LangfuseClient) GetProject(ctx context.Context) (string, string, error) {
	if c == nil {
		return "", "", fmt.Errorf("nil langfuse client")
	}

	url := strings.TrimRight(c.Host, "/") + "/api/public/projects"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", basicAuth(c.PublicKey, c.SecretKey))

	// per tenant by an operator (see entity.TenantLangfuse), not by
	// the requesting user.
	// codeql[go/request-forgery] False positive: c.Host is configured
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return "", "", ErrLangfuseUnauthorized
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return "", "", &LangfuseAPIError{StatusCode: res.StatusCode, Body: string(body)}
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", "", err
	}

	var parsed langfuseProjectsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", err
	}
	if len(parsed.Data) == 0 {
		return "", "", fmt.Errorf("langfuse: no project found")
	}
	return parsed.Data[0].ID, parsed.Data[0].Name, nil
}

// AuthCheck verifies the credentials are valid, mirroring the Python langfuse
// SDK's auth_check(). It returns (false, nil) when the credentials are
// rejected, and (false, err) for transport/remote errors.
func (c *LangfuseClient) AuthCheck(ctx context.Context) (bool, error) {
	_, _, err := c.GetProject(ctx)
	if err != nil {
		if errors.Is(err, ErrLangfuseUnauthorized) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
