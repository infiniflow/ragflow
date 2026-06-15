package core

import (
	"context"
	"fmt"

	"ragflow/internal/harness/core/schema"
)

// ---- ReAct run function ----

func (a *ReActAgent[M]) buildReActRunFunc() typedRunFunc[M] {
	return func(ctx context.Context, p *typedRunParams[M]) {
		maxIter := a.config.MaxIterations
		if maxIter <= 0 { maxIter = 10 }

		var state *TypedReActAgentState[M]
		if p.interruptState != nil { state = p.interruptState
		} else { state = NewReActAgentState(p.input.Messages, a.exeCtx.toolInfos, maxIter) }

		// Deep copy input messages to prevent middleware side-effects
		if len(state.Messages) > 0 {
			copied := make([]M, len(state.Messages))
			for i, m := range state.Messages { copied[i] = copyMessage(m) }
			state.Messages = copied
		}

		// Apply history modifier for resume
		if p.historyModifier != nil && len(state.Messages) > 0 {
			switch any(state.Messages[0]).(type) {
			case *schema.Message:
				msgs := make([]Message, len(state.Messages))
				for i, m := range state.Messages { msgs[i] = any(m).(Message) }
				modified := p.historyModifier(ctx, msgs)
				state.Messages = make([]M, len(modified))
				for i, m := range modified { state.Messages[i] = any(m).(M) }
			}
		}

		// BeforeAgent middlewares
		rc := &ReActAgentContext{Instruction: a.exeCtx.instruction, Tools: a.config.Tools, ReturnDirectly: a.exeCtx.returnDirectly, ToolSearchTool: a.exeCtx.toolSearchTool}
		if err := a.runBeforeAgent(&ctx, rc, p.generator); err != nil { return }

		model := BuildModelWrapperChain(a.config.Model, nil, a.config)

		var tn *ToolsNode[M]
		if a.config.ToolsConfig != nil {
			tn = NewToolsNode[M](a.config.ToolsConfig)
		} else if len(a.config.Tools) > 0 {
			// Auto-create ToolsNode from Tools list if ToolsConfig not set.
			tn = NewToolsNode[M](&ToolsNodeConfig{Tools: a.config.Tools})
		}

		for state.RemainingIterations > 0 {
			state.RemainingIterations--

			mc := &TypedModelContext[M]{Tools: state.ToolInfos, DeferredToolInfos: state.DeferredToolInfos, ModelRetryConfig: a.config.RetryConfig, ModelFailoverConfig: a.config.FailoverConfig}
			if err := a.runBeforeModelRewrite(&ctx, &state, mc, p.generator); err != nil { return }

			var modelMsgs []M
			if a.config.StateModifier != nil {
				var err error
				state, err = a.config.StateModifier(ctx, state)
				if err != nil { p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("StateModifier: %w", err)}); return }
			}

			if a.config.GenModelInput != nil {
				var err error
				modelMsgs, err = a.config.GenModelInput(ctx, rc.Instruction, &TypedAgentInput[M]{Messages: state.Messages})
				if err != nil {
					p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("GenModelInput: %w", err)})
					return
				}
			} else { modelMsgs = buildModelInputFromState(state.Messages, rc.Instruction) }

			resp, err := model.Generate(ctx, modelMsgs)
			if err != nil { p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("model: %w", err)}); return }
			p.generator.Send(typedModelOutputEvent(resp, nil))
			state.Messages = append(state.Messages, resp)

			if err := a.runAfterModelRewrite(&ctx, &state, mc, p.generator); err != nil { return }

			toolCalls := extractToolCalls(resp)
			if len(toolCalls) == 0 || tn == nil { break }

			var action *AgentAction
			results, act, err := tn.Execute(ctx, resp, state, nil)
			if err != nil { p.generator.Send(&TypedAgentEvent[M]{Err: err}); return }
			for _, tr := range results { state.Messages = append(state.Messages, tr) }
			action = act
			if action != nil && action.Exit { break }
		}

		if state.RemainingIterations <= 0 {
			p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("exceeded max iterations (%d)", a.config.MaxIterations)})
		}
		if a.config.OutputKey != "" && len(state.Messages) > 0 {
			if last := state.Messages[len(state.Messages)-1]; !isNilMessage(last) {
				setOutputToSession(ctx, last, a.config.OutputKey)
			}
		}
		if p.afterToolCallsHook != nil {
			if err := p.afterToolCallsHook(ctx); err != nil {
				p.generator.Send(&TypedAgentEvent[M]{Err: fmt.Errorf("after_tool_calls_hook: %w", err)})
			}
		}
		a.runAfterAgent(&ctx, state, p.generator)
	}
}
