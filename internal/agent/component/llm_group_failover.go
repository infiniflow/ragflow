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

package component

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	"ragflow/internal/agent/chat"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// chatGroupFailover carries what a gp- group invocation needs to fail over to
// the next member: the original reference, the tenant, and the pre-resolution
// driver/credential values so a flip re-runs resolution exactly as the first
// attempt did. An empty modelRef means the invocation does not target a model
// group; the failover helpers then collapse to a single attempt.
type chatGroupFailover struct {
	modelRef string
	tenantID string
	driver   string
	apiKey   string
	baseURL  string
}

// reresolveChatRequest re-resolves a gp- group reference after a flip and
// returns the prepared request pointed at the newly picked member's model
// name and credentials. Messages and sampling parameters are carried over
// unchanged — only the endpoint identity is swapped.
func reresolveChatRequest(ctx context.Context, db *gorm.DB, fo chatGroupFailover, base ChatInvokeRequest) (ChatInvokeRequest, error) {
	modelName, driver, apiKey, baseURL, info, err := resolveChatModelRefWithInfo(ctx, db, fo.modelRef, fo.driver, fo.apiKey, fo.baseURL)
	if err != nil {
		return ChatInvokeRequest{}, err
	}
	next := base
	next.ModelName, next.Driver, next.APIKey, next.BaseURL, next.ModelInfo = modelName, driver, apiKey, baseURL, info
	return next, nil
}

// invokeChatWithGroupFailover runs one chat invocation with gp- group
// failover: a retryable failure flips the currently picked member inactive
// and re-resolves the group reference to the next member. Non-group
// references collapse to a single attempt (the orchestrator never flips for
// them). It returns the response together with the request of the member
// that served it, so callers that re-invoke (structured-output retry) stay
// on the member that produced the result.
func invokeChatWithGroupFailover(
	ctx context.Context,
	db *gorm.DB,
	fo chatGroupFailover,
	inv ChatInvoker,
	req ChatInvokeRequest,
	canRetry func(error) bool,
) (*ChatInvokeResponse, ChatInvokeRequest, error) {
	usedReq := req
	var resp *ChatInvokeResponse
	err := dao.InvokeWithGroupFailover(ctx, db, fo.tenantID, entity.ModelTypeChat, fo.modelRef,
		func(context.Context) (ChatInvokeRequest, error) { return req, nil },
		func(ctx context.Context) (ChatInvokeRequest, error) { return reresolveChatRequest(ctx, db, fo, req) },
		func(ctx context.Context, attempt ChatInvokeRequest) error {
			var ierr error
			resp, ierr = inv.Invoke(ctx, db, attempt)
			if ierr == nil {
				usedReq = attempt
			}
			return ierr
		},
		canRetry,
	)
	return resp, usedReq, err
}

// streamChatWithGroupFailover performs one streaming chat invocation with
// gp- group failover. onDelta delivers one streaming delta to its
// destination; emit delivers a whole response when the invoker is not a
// StreamingInvoker. Returning an error from either aborts the current
// attempt. Failover only happens while nothing was delivered — once output
// reached the caller the output cannot be replayed on another model, so the
// error surfaces as-is.
func streamChatWithGroupFailover(
	ctx context.Context,
	db *gorm.DB,
	fo chatGroupFailover,
	inv ChatInvoker,
	req ChatInvokeRequest,
	onDelta func(delta string, isThink bool) error,
	emit func(thinking, content string) error,
) (*ChatInvokeResponse, ChatInvokeRequest, error) {
	delivered := false
	usedReq := req
	var resp *ChatInvokeResponse
	err := dao.InvokeWithGroupFailover(ctx, db, fo.tenantID, entity.ModelTypeChat, fo.modelRef,
		func(context.Context) (ChatInvokeRequest, error) { return req, nil },
		func(ctx context.Context) (ChatInvokeRequest, error) { return reresolveChatRequest(ctx, db, fo, req) },
		func(ctx context.Context, attempt ChatInvokeRequest) error {
			delivered = false
			var ierr error
			if streamer, ok := inv.(chat.StreamingInvoker); ok {
				resp, ierr = streamer.Stream(ctx, db, attempt, func(delta string, isThink bool) error {
					if derr := onDelta(delta, isThink); derr != nil {
						return derr
					}
					if delta != "" {
						delivered = true
					}
					return nil
				})
			} else {
				resp, ierr = inv.Invoke(ctx, db, attempt)
				if ierr == nil && resp != nil {
					if derr := emit(resp.Thinking, resp.Content); derr != nil {
						return derr
					}
					delivered = true
				}
			}
			if ierr == nil {
				usedReq = attempt
			}
			return ierr
		},
		func(error) bool { return !delivered },
	)
	return resp, usedReq, err
}

// agentModelGroup carries the wiring an Agent needs to fail over its chat
// model across the members of a gp- model group. It is nil on AgentParam
// when the model is not a group reference.
type agentModelGroup struct {
	modelRef string // original gp- reference
	tenantID string
	db       *gorm.DB
	// driver/apiKey/baseURL are the pre-resolution values so a flip re-runs
	// resolution exactly as the first attempt did.
	driver  string
	apiKey  string
	baseURL string
}

// groupFailoverChatModel wraps the Agent's ReAct chat model with model-group
// failover: when the current member's model call fails retryably, it flips
// the member inactive in the group and retries the next active member with a
// freshly resolved, freshly tool-bound chat model. Non-streaming Generate
// reuses dao.InvokeWithGroupFailover; Stream reuses it inside the goroutine
// that feeds a pipe, because the reader outlives the call, and only flips
// while no delta reached the caller.
type groupFailoverChatModel struct {
	inner model.ToolCallingChatModel // current member, tool-bound when tools were applied
	group agentModelGroup
	build func(ctx context.Context, driver, modelName, apiKey, baseURL string) (model.ToolCallingChatModel, error)
	tools []*schema.ToolInfo
	mu    sync.Mutex
}

func newGroupFailoverChatModel(
	inner model.ToolCallingChatModel,
	group agentModelGroup,
	build func(ctx context.Context, driver, modelName, apiKey, baseURL string) (model.ToolCallingChatModel, error),
) *groupFailoverChatModel {
	return &groupFailoverChatModel{inner: inner, group: group, build: build}
}

func (w *groupFailoverChatModel) current() model.ToolCallingChatModel {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.inner
}

func (w *groupFailoverChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := w.current().WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &groupFailoverChatModel{
		inner: bound,
		group: w.group,
		build: w.build,
		tools: tools,
	}, nil
}

func (w *groupFailoverChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	var msg *schema.Message
	err := dao.InvokeWithGroupFailover(ctx, w.group.db, w.group.tenantID, entity.ModelTypeChat, w.group.modelRef,
		func(context.Context) (model.ToolCallingChatModel, error) { return w.current(), nil },
		func(ctx context.Context) (model.ToolCallingChatModel, error) { return w.rebuildMember(ctx) },
		func(ctx context.Context, member model.ToolCallingChatModel) error {
			var gerr error
			msg, gerr = member.Generate(ctx, input, opts...)
			return gerr
		},
		nil)
	return msg, err
}

func (w *groupFailoverChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		delivered := false
		err := dao.InvokeWithGroupFailover(ctx, w.group.db, w.group.tenantID, entity.ModelTypeChat, w.group.modelRef,
			func(context.Context) (model.ToolCallingChatModel, error) { return w.current(), nil },
			func(ctx context.Context) (model.ToolCallingChatModel, error) { return w.rebuildMember(ctx) },
			func(ctx context.Context, member model.ToolCallingChatModel) error {
				return w.copyModelStream(ctx, member, input, writer, &delivered, opts)
			},
			func(error) bool { return !delivered && ctx.Err() == nil },
		)
		if err != nil {
			writer.Send(nil, err)
		}
	}()
	return reader, nil
}

// copyModelStream runs one member's stream attempt and forwards every delta
// to the pipe writer. It resets the delivered flag, so the failover gate
// always reflects the attempt that just failed.
func (w *groupFailoverChatModel) copyModelStream(
	ctx context.Context,
	member model.ToolCallingChatModel,
	input []*schema.Message,
	writer *schema.StreamWriter[*schema.Message],
	delivered *bool,
	opts []model.Option,
) error {
	*delivered = false
	stream, err := member.Stream(ctx, input, opts...)
	if err != nil {
		return err
	}
	defer stream.Close()
	for {
		message, rerr := stream.Recv()
		if errors.Is(rerr, io.EOF) {
			return nil
		}
		if rerr != nil {
			return rerr
		}
		if message == nil {
			continue
		}
		if writer.Send(message, nil) {
			// The caller stopped reading: stop the cascade rather than keep
			// pumping deltas no one will see.
			return context.Canceled
		}
		*delivered = true
	}
}

// rebuildMember re-resolves the group reference after a flip and builds the
// next member's chat model, re-binding the tools the ReAct agent applied to
// the previous member. Round accounting and retry configuration are carried
// over via withSwappedModel when the current member sits behind the round
// limiter.
func (w *groupFailoverChatModel) rebuildMember(ctx context.Context) (model.ToolCallingChatModel, error) {
	modelName, driver, apiKey, baseURL, err := resolveChatModelRef(ctx, w.group.db, w.group.modelRef, w.group.driver, w.group.apiKey, w.group.baseURL)
	if err != nil {
		return nil, err
	}
	member, err := w.build(ctx, driver, modelName, apiKey, baseURL)
	if err != nil {
		return nil, fmt.Errorf("build failover model: %w", err)
	}
	if w.tools != nil {
		bound, err := member.WithTools(w.tools)
		if err != nil {
			return nil, fmt.Errorf("bind tools on failover model: %w", err)
		}
		member = bound
	}
	if limiter, ok := w.current().(*roundLimitedChatModel); ok {
		member = limiter.withSwappedModel(member, modelName)
	}
	w.mu.Lock()
	w.inner = member
	w.mu.Unlock()
	return member, nil
}
