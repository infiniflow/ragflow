// Package pregel provides remote execution support for Pregel.
package pregel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ragflow/internal/harness/graph/types"
)

// RemoteRunnable executes nodes remotely via HTTP.
type RemoteRunnable struct {
	url        string
	headers    map[string]string
	httpClient *http.Client
	timeout    time.Duration
}

// RemoteConfig configures a remote runnable.
type RemoteConfig struct {
	URL        string
	Headers    map[string]string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// NewRemoteRunnable creates a new remote runnable.
func NewRemoteRunnable(config *RemoteConfig) *RemoteRunnable {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := config.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: timeout,
		}
	}

	headers := make(map[string]string)
	if config.Headers != nil {
		for k, v := range config.Headers {
			headers[k] = v
		}
	}

	return &RemoteRunnable{
		url:        config.URL,
		headers:    headers,
		httpClient: client,
		timeout:    timeout,
	}
}

// Execute sends a request to the remote server to execute a node.
func (r *RemoteRunnable) Execute(ctx context.Context, nodeName string, input interface{}, config *types.RunnableConfig) (interface{}, error) {
	// Build request
	reqBody := &RemoteExecuteRequest{
		Node:   nodeName,
		Input:  input,
		Config: config,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", r.url+"/execute", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}

	// Send request
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote execution failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result RemoteExecuteResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("remote execution error: %s", result.Error)
	}

	return result.Output, nil
}

// RemoteExecuteRequest represents a request to execute a node remotely.
type RemoteExecuteRequest struct {
	Node   string                 `json:"node"`
	Input  interface{}            `json:"input"`
	Config *types.RunnableConfig  `json:"config,omitempty"`
}

// RemoteExecuteResponse represents the response from a remote execution.
type RemoteExecuteResponse struct {
	Output interface{} `json:"output,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// PregelProtocol defines the protocol for remote Pregel execution.
type PregelProtocol interface {
	// Send sends a message to the remote peer.
	Send(ctx context.Context, message *PregelMessage) error
	// Receive receives a message from the remote peer.
	Receive(ctx context.Context) (*PregelMessage, error)
	// Close closes the protocol connection.
	Close() error
}

// PregelMessage represents a message in the Pregel protocol.
type PregelMessage struct {
	Type      MessageType            `json:"type"`
	ID        string                 `json:"id"`
	NodeName  string                 `json:"node_name,omitempty"`
	Input     interface{}            `json:"input,omitempty"`
	Output    interface{}            `json:"output,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// MessageType represents the type of a Pregel message.
type MessageType string

const (
	// MessageTypeExecute requests node execution.
	MessageTypeExecute MessageType = "execute"
	// MessageTypeExecuteResponse is the response to an execute request.
	MessageTypeExecuteResponse MessageType = "execute_response"
	// MessageTypeCheckpoint sends checkpoint data.
	MessageTypeCheckpoint MessageType = "checkpoint"
	// MessageTypeStateUpdate sends state updates.
	MessageTypeStateUpdate MessageType = "state_update"
	// MessageTypeInterrupt sends interrupt information.
	MessageTypeInterrupt MessageType = "interrupt"
	// MessageTypeResume resumes execution from interrupt.
	MessageTypeResume MessageType = "resume"
	// MessageTypePing is a heartbeat.
	MessageTypePing MessageType = "ping"
	// MessageTypePong is a heartbeat response.
	MessageTypePong MessageType = "pong"
)

// HTTPPregelProtocol implements PregelProtocol over HTTP.
type HTTPPregelProtocol struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

// NewHTTPPregelProtocol creates a new HTTP Pregel protocol.
func NewHTTPPregelProtocol(baseURL string, headers map[string]string) *HTTPPregelProtocol {
	return &HTTPPregelProtocol{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		headers:    headers,
	}
}

// Send sends a message to the remote peer.
func (p *HTTPPregelProtocol) Send(ctx context.Context, message *PregelMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/pregel/message", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Receive receives a message from the remote peer.
func (p *HTTPPregelProtocol) Receive(ctx context.Context) (*PregelMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/pregel/message", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to receive message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil // No message available
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("receive failed with status %d: %s", resp.StatusCode, string(body))
	}

	var message PregelMessage
	if err := json.NewDecoder(resp.Body).Decode(&message); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}

	return &message, nil
}

// Close closes the protocol connection.
func (p *HTTPPregelProtocol) Close() error {
	return nil
}

// RemoteNode wraps a remote execution capability as a node function.
type RemoteNode struct {
	runnable *RemoteRunnable
	nodeName string
}

// NewRemoteNode creates a new remote node.
func NewRemoteNode(runnable *RemoteRunnable, nodeName string) *RemoteNode {
	return &RemoteNode{
		runnable: runnable,
		nodeName: nodeName,
	}
}

// Execute executes the remote node.
func (n *RemoteNode) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	return n.runnable.Execute(ctx, n.nodeName, input, nil)
}

// NodeToRemoteRunnable converts a local node to a remote runnable.
func NodeToRemoteRunnable(node types.NodeFunc, url string) *RemoteRunnable {
	// This would register the node locally and expose it via HTTP
	// Implementation depends on the HTTP server setup
	return NewRemoteRunnable(&RemoteConfig{
		URL: url,
	})
}
