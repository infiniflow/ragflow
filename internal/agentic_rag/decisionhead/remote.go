// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package decisionhead

// remote.go runs the decision-head graph in a sidecar process.
//
// It exists because the ONNX Runtime linked into this binary is an ORT-format
// minimal build that covers DeepDoc's operators and not the head's: session creation
// fails at Shape(15). Fixing that is a change to how the native library is
// built, so until then the graph runs where a full ONNX Runtime is available and
// this process only assembles ids and reads back logits.
//
// It is deliberately a thin client. The tokenizer, the contract, the templates,
// the language routing, the temperature and the threshold all stay in Go — the
// sidecar is handed a finished sequence and returns numbers. That way the
// contract has exactly one implementation, and the in-process path (the same
// scorer interface, a *Session instead of this) remains the destination: nothing
// outside open() knows which of the two is running.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxJudgeResponse bounds what a broken sidecar can make this process allocate.
// A response is one float per option.
const maxJudgeResponse = 1 << 20

// remoteScorer posts assembled sequences to a decision-head sidecar.
//
// Single-flight is not enforced here: the sidecar serialises inference behind
// its own lock, because the model's ~2 GB of activations are the sidecar's, not
// this process's. Adding a second gate on this side would only queue requests
// twice.
type remoteScorer struct {
	url    string
	client *http.Client
}

func newRemoteScorer(url string) *remoteScorer {
	return &remoteScorer{
		url: url,
		// No client timeout: the judging budget and the request context already
		// bound the call, and a second, shorter deadline here would turn a slow
		// but valid decision into a dropped chunk.
		client: &http.Client{},
	}
}

type judgeRequest struct {
	InputIDs  []int32 `json:"input_ids"`
	MarkerPos []int32 `json:"marker_pos"`
	QType     int64   `json:"qtype"`
}

type judgeResponse struct {
	Logits []float32 `json:"logits"`
	Error  string    `json:"error"`
}

// Logits sends seq to the sidecar and returns its per-option logits.
func (r *remoteScorer) Logits(ctx context.Context, seq Sequence) ([]float32, error) {
	payload, err := json.Marshal(judgeRequest{
		InputIDs:  seq.IDs,
		MarkerPos: seq.Markers,
		QType:     seq.QType,
	})
	if err != nil {
		return nil, fmt.Errorf("decisionhead: encoding sequence: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+"/judge", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("decisionhead: judge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: judge request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJudgeResponse))
	if err != nil {
		return nil, fmt.Errorf("decisionhead: reading judge response: %w", err)
	}
	var out judgeResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decisionhead: decoding judge response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The sidecar reports its own failures in the body; keep it verbatim,
		// it is the only thing that says why the graph refused to run.
		return nil, fmt.Errorf("decisionhead: judge status %d: %s", resp.StatusCode, out.Error)
	}
	if len(out.Logits) == 0 {
		return nil, fmt.Errorf("decisionhead: judge returned no logits")
	}
	return out.Logits, nil
}

// probeSidecar checks the sidecar is up and has its model loaded, so a
// deployment that points at a dead or empty sidecar logs that once at load
// instead of degrading silently on every chunk.
func probeSidecar(ctx context.Context, client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJudgeResponse))
	if err != nil {
		return err
	}
	var health struct {
		Ready bool   `json:"ready"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &health); err != nil {
		return fmt.Errorf("health: %w", err)
	}
	if !health.Ready {
		return fmt.Errorf("model not loaded: %s", health.Error)
	}
	return nil
}

// probeTimeout bounds the startup health check. It is separate from the judging
// budget: loading a 1.7 GB graph can take a while, but a probe that hangs for
// the whole budget would stall the first request behind it.
const probeTimeout = 5 * time.Second
