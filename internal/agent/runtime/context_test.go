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

package runtime

import (
	"context"
	"testing"
	"time"
)

func TestAgentMessageCallbacksRunOutsideStateLocks(t *testing.T) {
	assertCompletes := func(name string, fn func()) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			fn()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("%s callback re-entered a locked state", name)
		}
	}

	var agentCtx context.Context
	agentCtx = WithAgentMessageEmitter(context.Background(), func(string, string) {
		_ = AgentMessageEventsEmitted(agentCtx)
	})
	assertCompletes("Agent emitter", func() { EmitAgentMessage(agentCtx, "token", "") })

	var sinkCtx context.Context
	sinkCtx = WithAgentDeltaSink(context.Background(), func(string, string) {
		_ = DeferredAgentMessageEventsEmitted(sinkCtx)
	})
	assertCompletes("deferred sink", func() { EmitAgentMessage(sinkCtx, "token", "") })

	var canvasCtx context.Context
	canvasCtx = WithAgentMessageEmitter(context.Background(), func(string, string) {})
	canvasCtx = WithCanvasMessageEmitter(canvasCtx, func(string) {
		_ = AgentMessageEventsEmitted(canvasCtx)
	})
	assertCompletes("Canvas emitter", func() { EmitCanvasMessage(canvasCtx, "message") })

	var lifecycleCtx context.Context
	lifecycleCtx = WithAgentMessageEmitterControl(
		context.Background(),
		func(string, string) {},
		func() bool {
			_ = AgentMessageEventsEmitted(lifecycleCtx)
			return false
		},
		func() { _ = AgentMessageEventsEmitted(lifecycleCtx) },
	)
	assertCompletes("finalize", func() { FinalizeAgentMessage(lifecycleCtx) })
	assertCompletes("reset", func() { ResetAgentMessageEmission(lifecycleCtx) })
}

func TestAgentMessageRunStateSurvivesInvocationReset(t *testing.T) {
	ctx := WithAgentMessageEmitter(context.Background(), func(string, string) {})
	EmitAgentMessage(ctx, "first", "")
	ResetAgentMessageEmission(ctx)

	if AgentMessageEventsEmitted(ctx) {
		t.Fatal("invocation emitted state survived reset")
	}
	if !AgentMessageEventsEmittedRun(ctx) {
		t.Fatal("run emitted state was cleared by invocation reset")
	}

	suppressedCtx := WithComponentExecutionOptions(ctx, ComponentExecutionOptions{SuppressAgentMessageEvents: true})
	EmitAgentMessage(suppressedCtx, "hidden", "")
	ResetAgentMessageEmission(ctx)
	if !AgentMessageEventsSuppressedRun(ctx) {
		t.Fatal("run suppressed state was cleared by invocation reset")
	}
}
