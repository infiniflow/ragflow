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
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/nlp"
)

// scriptedJSONDriver fakes the DeepResearcher LLM calls: sufficiency checks
// report "insufficient" and query generation returns two sub-queries, so the
// recursion spawns parallel sub-research goroutines. Like a real driver, it
// fails once the request context is canceled.
type scriptedJSONDriver struct {
	*modelModule.DummyModel
}

func (d *scriptedJSONDriver) ChatWithMessages(
	ctx context.Context,
	modelName string,
	messages []modelModule.Message,
	apiConfig *modelModule.APIConfig,
	chatModelConfig *modelModule.ChatConfig,
	modelUsage *common.ModelUsage,
) (*modelModule.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	system := ""
	if len(messages) > 0 {
		if s, ok := messages[0].Content.(string); ok {
			system = s
		}
	}
	answer := `{"is_sufficient": false, "reasoning": "need more", "missing_information": ["detail"]}`
	if strings.Contains(system, "query optimization expert") {
		answer = `{"reasoning": "split", "questions": [{"question": "sub one", "query": "one"}, {"question": "sub two", "query": "two"}]}`
	}
	return &modelModule.ChatResponse{Answer: &answer}, nil
}

// TestDeepResearcherCancelDrainsSubResearch verifies that when the context is
// canceled mid-research, Research does not return while sub-research
// goroutines are still running. Regression for the "panic: send on closed
// channel" crash: the chat pipeline closed its output channel after Research
// returned, while orphaned sub-research goroutines were still invoking the
// progress callback and sending on it, killing the whole api server.
func TestDeepResearcherCancelDrainsSubResearch(t *testing.T) {
	modelName := "fake-model"
	var retrieveCalls atomic.Int32
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	dr := NewDeepResearcher(
		&modelModule.ChatModel{
			ModelDriver: &scriptedJSONDriver{DummyModel: modelModule.NewDummyModel(nil, modelModule.URLSuffix{})},
			ModelName:   &modelName,
			APIConfig:   &modelModule.APIConfig{},
		},
		map[string]interface{}{},
		func(ctx context.Context, question string) (*nlp.RetrievalResult, error) {
			// The first retrieval (top level) completes immediately so the
			// recursion reaches the parallel step; sub-research retrievals
			// park here until the test releases them after cancellation.
			if retrieveCalls.Add(1) > 1 {
				entered <- struct{}{}
				<-release
			}
			return &nlp.RetrievalResult{}, nil
		},
		false, nil, nil, nil, nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var returned, violated, callbacks atomic.Bool
	callback := func(msg string) {
		if returned.Load() {
			violated.Store(true)
		}
		callbacks.Store(true)
	}

	done := make(chan error, 1)
	go func() {
		err := dr.Research(ctx, map[string]interface{}{}, "question", "query", callback)
		returned.Store(true)
		done <- err
	}()

	// Wait for both parallel sub-research goroutines to park in retrieval,
	// then cancel the request mid-flight and let them resume.
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for sub-research goroutines to reach retrieval")
		}
	}
	cancel()
	close(release)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Research did not return after cancellation; sub-research goroutines never drained")
	}

	if violated.Load() {
		t.Fatal("progress callback fired after Research returned")
	}
	// The tree may still be winding down through the driver error path;
	// after a grace period nothing may invoke the callback anymore.
	time.Sleep(300 * time.Millisecond)
	if returned.Load() && violated.Load() {
		t.Fatal("late progress callback detected after Research returned")
	}
}

func TestDeepResearchProgressCallbackDeliversMarkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan AsyncChatResult, 4)
	cb := (&ChatPipelineService{}).deepResearchProgressCallback(ctx, out)

	cb("<START_DEEP_RESEARCH>")
	cb("Retrieval 3 results in 12.0ms")
	cb("<END_DEEP_RESEARCH>")

	want := []string{"<retrieving>", "Retrieval 3 results in 12.0ms", "</retrieving>"}
	for i, exp := range want {
		select {
		case r := <-out:
			if r.Answer != exp {
				t.Fatalf("result[%d].Answer = %q, want %q", i, r.Answer, exp)
			}
			if r.Final {
				t.Fatalf("result[%d] must not be final", i)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for result %d", i)
		}
	}
}

func TestDeepResearchProgressCallbackDropsAfterConsumerGone(t *testing.T) {
	// Unbuffered channel with no reader: the consumer is gone.
	out := make(chan AsyncChatResult)
	ctx, cancel := context.WithCancel(context.Background())
	cb := (&ChatPipelineService{}).deepResearchProgressCallback(ctx, out)

	cancel()
	done := make(chan struct{})
	go func() {
		cb("Retrieval 3 results in 12.0ms")
		close(done)
	}()
	select {
	case <-done: // dropped instead of blocking or panicking
	case <-time.After(2 * time.Second):
		t.Fatal("progress callback blocked after the consumer was gone")
	}
	select {
	case r := <-out:
		t.Fatalf("unexpected delivery on consumer-gone channel: %+v", r)
	default:
	}
}
