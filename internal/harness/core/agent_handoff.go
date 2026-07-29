package core

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"ragflow/internal/harness/core/schema"
)

// GenTransferInstruction generates an instruction string for agent transfer.
func GenTransferInstruction(names []string) string {
	if len(names) == 0 {
		return ""
	}
	s := "You can transfer to the following agents:\n"
	for _, n := range names {
		s += fmt.Sprintf("- %s\n", n)
	}
	return s
}

// GenToolInstruction generates tool instruction for an agent.
func GenToolInstruction(name, desc string) string {
	return fmt.Sprintf("Agent '%s': %s", name, desc)
}

// exactRunPathMatch checks if two run paths are exactly equal.
// This prevents sub-agents from forging paths to access restricted agents.
func exactRunPathMatch(a, b []RunStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}

func init() {
	schema.RegisterName[*deterministicTransferState]("_harness_deterministic_transfer_state")
}

// deterministicTransferState holds event history for deterministic transfer resume.
type deterministicTransferState struct {
	EventList []any
}

// DeterministicTransferConfig configures deterministic transfer.
type DeterministicTransferConfig struct {
	Agent        Agent
	ToAgentNames []string
}

// AgentWithDeterministicTransfer wraps an agent to transfer to given agents deterministically.
func AgentWithDeterministicTransfer(_ context.Context, config *DeterministicTransferConfig) Agent {
	if ra, ok := config.Agent.(ResumableAgent); ok {
		return &resumableAgentWithDeterministicTransfer{
			agent:        ra,
			toAgentNames: config.ToAgentNames,
		}
	}
	return &agentWithDeterministicTransfer{
		agent:        config.Agent,
		toAgentNames: config.ToAgentNames,
	}
}

type agentWithDeterministicTransfer struct {
	agent        Agent
	toAgentNames []string
}

func (a *agentWithDeterministicTransfer) Description(ctx context.Context) string {
	return a.agent.Description(ctx)
}
func (a *agentWithDeterministicTransfer) Name(ctx context.Context) string { return a.agent.Name(ctx) }
func (a *agentWithDeterministicTransfer) GetType() string                 { return "DeterministicTransfer" }

func (a *agentWithDeterministicTransfer) Run(ctx context.Context, input *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	if fa, ok := a.agent.(*flowAgent); ok {
		return runFlowAgentWithIsolatedSession(ctx, fa, input, a.toAgentNames, opts...)
	}
	aIter := a.agent.Run(ctx, input, opts...)
	iterator, generator := NewAsyncIteratorPair[*AgentEvent]()
	go forwardEventsAndAppendTransfer(aIter, generator, a.toAgentNames)
	return iterator
}

type resumableAgentWithDeterministicTransfer struct {
	agent        ResumableAgent
	toAgentNames []string
}

func (a *resumableAgentWithDeterministicTransfer) Description(ctx context.Context) string {
	return a.agent.Description(ctx)
}
func (a *resumableAgentWithDeterministicTransfer) Name(ctx context.Context) string {
	return a.agent.Name(ctx)
}
func (a *resumableAgentWithDeterministicTransfer) GetType() string { return "DeterministicTransfer" }

func (a *resumableAgentWithDeterministicTransfer) Run(ctx context.Context, input *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	if fa, ok := a.agent.(*flowAgent); ok {
		return runFlowAgentWithIsolatedSession(ctx, fa, input, a.toAgentNames, opts...)
	}
	aIter := a.agent.Run(ctx, input, opts...)
	iterator, generator := NewAsyncIteratorPair[*AgentEvent]()
	go forwardEventsAndAppendTransfer(aIter, generator, a.toAgentNames)
	return iterator
}

func (a *resumableAgentWithDeterministicTransfer) Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	if fa, ok := a.agent.(*flowAgent); ok {
		return resumeFlowAgentWithIsolatedSession(ctx, fa, info, a.toAgentNames, opts...)
	}
	aIter := a.agent.Resume(ctx, info, opts...)
	iterator, generator := NewAsyncIteratorPair[*AgentEvent]()
	go forwardEventsAndAppendTransfer(aIter, generator, a.toAgentNames)
	return iterator
}

func forwardEventsAndAppendTransfer(iter *AsyncIterator[*AgentEvent], generator *AsyncGenerator[*AgentEvent], toAgentNames []string) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			generator.Send(&AgentEvent{Err: fmt.Errorf("panic: %v\n%s", panicErr, debug.Stack())})
		}
		generator.Close()
	}()

	var lastEvent *AgentEvent
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		generator.Send(event)
		lastEvent = event
	}

	if lastEvent != nil && lastEvent.Action != nil && (lastEvent.Action.Interrupted != nil || lastEvent.Action.Exit) {
		return
	}
	sendTransferEvents(generator, toAgentNames)
}

func runFlowAgentWithIsolatedSession(ctx context.Context, fa *flowAgent, input *AgentInput, toAgentNames []string, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	parentSession := getSession(ctx)
	parentRunCtx := getRunCtx(ctx)

	isolatedSession := &runSession{
		Values:   make(map[string]any),
		valuesMx: &sync.Mutex{},
	}
	if parentSession != nil {
		isolatedSession.Values = parentSession.Values
		isolatedSession.valuesMx = parentSession.valuesMx
	}

	rootInput := input
	if parentRunCtx != nil {
		if r, ok := parentRunCtx.RootInput.(*AgentInput); ok && r != nil {
			rootInput = r
		}
	}
	var runPath []RunStep
	if parentRunCtx != nil {
		runPath = parentRunCtx.getRunPath()
	}

	ctx = setRunCtx(ctx, &runContext{
		RootInput: rootInput,
		RunPath:   runPath,
		Session:   isolatedSession,
	})

	iter := fa.Run(ctx, input, opts...)

	iterator, generator := NewAsyncIteratorPair[*AgentEvent]()
	go handleFlowAgentEvents(ctx, iter, generator, isolatedSession, parentSession, toAgentNames)
	return iterator
}

func resumeFlowAgentWithIsolatedSession(ctx context.Context, fa *flowAgent, info *ResumeInfo, toAgentNames []string, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	state, ok := info.InterruptState.(*deterministicTransferState)
	if !ok || state == nil {
		eIter, eGen := NewAsyncIteratorPair[*AgentEvent]()
		eGen.Send(&AgentEvent{Err: errors.New("invalid interrupt state for flowAgent resume in deterministic transfer")})
		eGen.Close()
		return eIter
	}

	parentSession := getSession(ctx)
	parentRunCtx := getRunCtx(ctx)

	isolatedSession := &runSession{
		Values:   make(map[string]any),
		valuesMx: &sync.Mutex{},
	}
	if parentSession != nil {
		isolatedSession.Values = parentSession.Values
		isolatedSession.valuesMx = parentSession.valuesMx
	}
	// Restore events from deterministic transfer state
	for _, ev := range state.EventList {
		isolatedSession.addEvent(ev)
	}

	rootInput := any(nil)
	if parentRunCtx != nil {
		rootInput = parentRunCtx.RootInput
	}
	var runPath []RunStep
	if parentRunCtx != nil {
		runPath = parentRunCtx.getRunPath()
	}

	ctx = setRunCtx(ctx, &runContext{
		RootInput: rootInput,
		RunPath:   runPath,
		Session:   isolatedSession,
	})

	iter := fa.Resume(ctx, info, opts...)

	iterator, generator := NewAsyncIteratorPair[*AgentEvent]()
	go handleFlowAgentEvents(ctx, iter, generator, isolatedSession, parentSession, toAgentNames)
	return iterator
}

func handleFlowAgentEvents(ctx context.Context, iter *AsyncIterator[*AgentEvent], generator *AsyncGenerator[*AgentEvent], isolatedSession, parentSession *runSession, toAgentNames []string) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			generator.Send(&AgentEvent{Err: fmt.Errorf("panic: %v\n%s", panicErr, debug.Stack())})
		}
		generator.Close()
	}()

	var lastEvent *AgentEvent

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if parentSession != nil && (event.Action == nil || event.Action.Interrupted == nil) {
			copied := copyTypedAgentEvent(event)
			setAutomaticClose(copied)
			setAutomaticClose(event)
			parentSession.addEvent(copied)
		}

		if event.Action != nil && event.Action.internalInterrupted != nil {
			lastEvent = event
			continue
		}

		generator.Send(event)
		lastEvent = event
	}

	if lastEvent != nil && lastEvent.Action != nil {
		if lastEvent.Action.internalInterrupted != nil {
			events := isolatedSession.getEvents()
			state := &deterministicTransferState{EventList: events}
			compositeEvent := CompositeInterrupt(ctx, "deterministic transfer wrapper interrupted", state, lastEvent.Action.internalInterrupted)
			generator.Send(compositeEvent)
			return
		}
		if lastEvent.Action.Exit {
			return
		}
	}
	sendTransferEvents(generator, toAgentNames)
}

func sendTransferEvents(generator *AsyncGenerator[*AgentEvent], toAgentNames []string) {
	for _, toAgentName := range toAgentNames {
		aMsg, tMsg := GenTransferMessages(context.Background(), toAgentName)
		aEvent := EventFromMessage(aMsg, nil, schema.RoleAssistant, "")
		generator.Send(aEvent)
		tEvent := EventFromMessage(tMsg, nil, schema.RoleTool, tMsg.Name)
		tEvent.Action = &AgentAction{
			TransferToAgent: &TransferToAgentAction{
				DestAgentName: toAgentName,
			},
		}
		generator.Send(tEvent)
	}
}

// GenTransferMessages creates a pair of messages for agent transfer.
func GenTransferMessages(ctx context.Context, agentName string) (*schema.Message, *schema.Message) {
	transferring := "Transferring to " + agentName + "..."
	msg := &schema.Message{
		Role:    schema.RoleAssistant,
		Content: transferring,
	}
	transferFuncName := "transfer_to_" + agentName
	toolMsg := &schema.Message{
		Role:     schema.RoleTool,
		Content:  `{"agent":"` + agentName + `"}`,
		Name:     agentName,
		ToolName: transferFuncName,
	}
	return msg, toolMsg
}

// ---- Message ID utilities (ported from ADK internal/message_id.go) ----

const EinoMsgIDKey = "_eino_msg_id"

func GetMessageID(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	id, _ := extra[EinoMsgIDKey].(string)
	return id
}

func SetMessageID(extra map[string]any, id string) map[string]any {
	if extra == nil {
		extra = make(map[string]any)
	}
	extra[EinoMsgIDKey] = id
	return extra
}

func EnsureMessageID(extra map[string]any) map[string]any {
	if GetMessageID(extra) != "" {
		return extra
	}
	return SetMessageID(extra, uuidV4())
}

func uuidV4() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
