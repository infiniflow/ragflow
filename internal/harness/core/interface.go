package core

import (
	"bytes"
	"context"
	"encoding/gob"
	"io"

	"ragflow/internal/harness/core/schema"
)

func init() {
	gob.Register(&RunStep{})
}

// MessageType is the sealed type constraint for agent message types.
type MessageType interface {
	*schema.Message | *schema.AgenticMessage
}

// ===== Type aliases =====
type Message = *schema.Message
type MessageStream = *schema.StreamReader[Message]
type AgenticMessage = *schema.AgenticMessage
type AgenticMessageStream = *schema.StreamReader[AgenticMessage]

// ===== Agent action =====

type TransferToAgentAction struct {
	DestAgentName string
}

func NewTransferToAgentAction(dest string) *AgentAction {
	return &AgentAction{TransferToAgent: &TransferToAgentAction{DestAgentName: dest}}
}

func NewExitAction() *AgentAction {
	return &AgentAction{Exit: true}
}

type BreakLoopAction struct {
	From              string
	Done              bool
	CurrentIterations int
}

func NewBreakLoopAction(agentName string) *AgentAction {
	return &AgentAction{BreakLoop: &BreakLoopAction{From: agentName}}
}

type AgentAction struct {
	Exit                bool
	Interrupted         *InterruptInfo
	TransferToAgent     *TransferToAgentAction
	BreakLoop           *BreakLoopAction
	CustomizedAction    any
	internalInterrupted *InterruptSignal
}

// ===== Run step =====

type RunStep struct {
	agentName string
}

func NewRunStep(agentName string) *RunStep { return &RunStep{agentName: agentName} }
func (r *RunStep) String() string          { return r.agentName }
func (r *RunStep) Equals(r1 RunStep) bool  { return r.agentName == r1.agentName }

// GobEncode implements gob.GobEncoder for checkpoint serialization.
func (r *RunStep) GobEncode() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(r.agentName); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GobDecode implements gob.GobDecoder for checkpoint deserialization.
func (r *RunStep) GobDecode(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(&r.agentName)
}

// ===== Events =====

type TypedMessageVariant[M MessageType] struct {
	IsStreaming   bool
	Message       M
	MessageStream *schema.StreamReader[M]
	Role          schema.RoleType
	AgenticRole   schema.AgenticRoleType
	ToolName      string
}

func (mv *TypedMessageVariant[M]) GetMessage() (M, error) {
	if mv.IsStreaming {
		return concatMessageStream(mv.MessageStream)
	}
	return mv.Message, nil
}

type MessageVariant = TypedMessageVariant[*schema.Message]

type TypedAgentOutput[M MessageType] struct {
	MessageOutput    *TypedMessageVariant[M]
	CustomizedOutput any
}

type AgentOutput = TypedAgentOutput[*schema.Message]

type TypedAgentEvent[M MessageType] struct {
	AgentName string
	RunPath   []RunStep
	Output    *TypedAgentOutput[M]
	Action    *AgentAction
	Err       error
}

type AgentEvent = TypedAgentEvent[*schema.Message]

type TypedAgentInput[M MessageType] struct {
	Messages        []M
	EnableStreaming bool
}

type AgentInput = TypedAgentInput[*schema.Message]

// ===== Agent interfaces =====

type TypedAgent[M MessageType] interface {
	Name(ctx context.Context) string
	Description(ctx context.Context) string
	Run(ctx context.Context, input *TypedAgentInput[M], opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]]
}

type Agent = TypedAgent[*schema.Message]


type TypedResumableAgent[M MessageType] interface {
	TypedAgent[M]
	Resume(ctx context.Context, info *ResumeInfo, opts ...RunOption) *AsyncIterator[*TypedAgentEvent[M]]
}

type ResumableAgent = TypedResumableAgent[*schema.Message]

// ===== Event constructors =====

func EventFromMessage(msg Message, msgStream MessageStream, role schema.RoleType, toolName string) *AgentEvent {
	return typedEventFromMessage(msg, msgStream, role, toolName)
}

func typedEventFromMessage[M MessageType](msg M, msgStream *schema.StreamReader[M], role schema.RoleType, toolName string) *TypedAgentEvent[M] {
	return &TypedAgentEvent[M]{
		Output: &TypedAgentOutput[M]{
			MessageOutput: &TypedMessageVariant[M]{
				IsStreaming: msgStream != nil, Message: msg, MessageStream: msgStream,
				Role: role, ToolName: toolName,
			},
		},
	}
}

func typedModelOutputEvent[M MessageType](msg M, msgStream *schema.StreamReader[M]) *TypedAgentEvent[M] {
	var role schema.RoleType
	var agenticRole schema.AgenticRoleType
	var zero M
	if _, ok := any(zero).(*schema.Message); ok {
		role = schema.RoleAssistant
	} else {
		agenticRole = schema.AgenticRoleAssistant
	}
	event := typedEventFromMessage(msg, msgStream, role, "")
	event.Output.MessageOutput.AgenticRole = agenticRole
	return event
}

func EventFromAgenticMessage(msg AgenticMessage, msgStream AgenticMessageStream, agenticRole schema.AgenticRoleType) *TypedAgentEvent[*schema.AgenticMessage] {
	return &TypedAgentEvent[*schema.AgenticMessage]{
		Output: &TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &TypedMessageVariant[*schema.AgenticMessage]{
				IsStreaming: msgStream != nil, Message: msg, MessageStream: msgStream,
				AgenticRole: agenticRole,
			},
		},
	}
}

// ===== Utilities =====

func isNilMessage[M MessageType](msg M) bool {
	var zero M
	return any(msg) == any(zero)
}

func concatMessageStream[M MessageType](stream *schema.StreamReader[M]) (M, error) {
	var zero M
	switch s := any(stream).(type) {
	case *schema.StreamReader[*schema.Message]:
		result, err := schema.ConcatMessageStream(s)
		if err != nil {
			return zero, err
		}
		return any(result).(M), nil
	case *schema.StreamReader[*schema.AgenticMessage]:
		defer s.Close()
		var msgs []*schema.AgenticMessage
		for {
			frame, err := s.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return zero, err
			}
			msgs = append(msgs, frame)
		}
		result, err := schema.ConcatAgenticMessages(msgs)
		if err != nil {
			return zero, err
		}
		return any(result).(M), nil
	default:
		panic("unreachable: unknown MessageType")
	}
}

// typedModelOption is a model option with a function.
type typedModelOption[M MessageType] struct {
	f func(o *modelOptions[M])
}
func (o *typedModelOption[M]) applyModel() {}

// modelOptions holds all model call options.
type modelOptions[M MessageType] struct {
	RetryConfig *TypedModelRetryConfig[M]
}

func init() {
	schema.RegisterType("agentcore_run_step", func() any { return &RunStep{} })
	schema.RegisterType("agentcore_event", func() any { return &TypedAgentEvent[*schema.Message]{} })
}
