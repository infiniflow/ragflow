package agentic_rag

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
)

// ActionService is the boundary between the tools and the action layer.
//
// It is an interface rather than a direct dependency because internal/service
// imports agentic_rag (the chat pipeline drives the agent), so the agent cannot
// import it back. The adapter is installed at boot, exactly like the ontology
// service, and a deployment that never configures a binding keeps the stub.
//
// The two methods are deliberately asymmetric. Read returns the decision AND the
// reasons; Invoke returns the decision and, when it says no, does not call
// anything. The agent never evaluates a gate itself — it is told the answer.
type ActionService interface {
	// ActionContract returns one action's contract, binding state and verdict.
	ActionContract(ctx context.Context, req ActionContractRequest) (*ActionContractResult, error)
	// InvokeAction runs the action when the server-side verdict allows it.
	InvokeAction(ctx context.Context, req ActionInvokeRequest) (*ActionInvokeResult, error)
}

// ActionContractRequest asks about one action.
type ActionContractRequest struct {
	UserID     string
	ModelID    string
	DatasetIDs []string
	ActionName string
}

// ActionContractResult is the judged contract.
type ActionContractResult struct {
	ActionName string
	Verdict    string
	Reasons    []string
	// Fields maps the six contract field names to their judged state.
	Fields  map[string]ActionContractField
	Missing []string
	// Required is the declaration this contract is judged against: the fields the
	// model was actually asked for. A template may narrow the contract, and a field
	// it never asked about is not owed.
	Required []string
	// Declared is how many of the required fields have a value, for the "4/6"
	// summary.
	Declared int
	// Bound and BoundReadOnly describe the binding; they are what makes a
	// "why is this manual" answer complete.
	Bound          bool
	BindingEnabled bool
	ReadOnly       bool
}

// ActionContractField is one field's judgement.
type ActionContractField struct {
	Status     string
	Value      any
	Quote      string
	Provenance string
}

// ActionInvokeRequest asks to run one action.
type ActionInvokeRequest struct {
	UserID     string
	ModelID    string
	DatasetIDs []string
	ActionName string
	// Arguments are the model's proposed tool arguments. They override the
	// binding's mapping for those keys only; they cannot introduce a key the
	// mapping does not allow at the transport layer, but they can carry a value
	// the corpus cannot supply (an account number the user typed).
	Arguments map[string]any
	// TurnID links the audit row to the conversation turn.
	TurnID string
}

// ActionInvokeResult is what happened.
type ActionInvokeResult struct {
	Verdict    string
	Reasons    []string
	Executed   bool
	Status     string
	Output     string
	Error      string
	DurationMS int
}

// ErrActionServiceMissing is returned when no action service was installed.
var ErrActionServiceMissing = errors.New("agentic_rag: no action service installed")

// actionExecEnvKey is the deployment switch for action execution.
//
// The agent reads it only to decide whether to offer the tool at all; the
// authoritative check happens server-side on every call. Duplicating one env
// lookup is the price of not importing the service package here, and it is safe
// in the only direction that matters: if the two ever disagree, the server still
// refuses.
const actionExecEnvKey = "RAGFLOW_ONTOLOGY_ACTION_EXEC"

// ActionExecEnabledForTools reports whether the invoke tool may be offered.
func ActionExecEnabledForTools() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(actionExecEnvKey))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

var (
	actionServiceMu   sync.RWMutex
	actionServiceImpl ActionService = stubActionService{}
)

// SetActionService installs the action service. nil restores the stub.
func SetActionService(svc ActionService) {
	actionServiceMu.Lock()
	defer actionServiceMu.Unlock()
	if svc == nil {
		actionServiceImpl = stubActionService{}
		return
	}
	actionServiceImpl = svc
}

// GetActionService returns the installed action service.
func GetActionService() ActionService {
	actionServiceMu.RLock()
	defer actionServiceMu.RUnlock()
	return actionServiceImpl
}

type stubActionService struct{}

func (stubActionService) ActionContract(context.Context, ActionContractRequest) (*ActionContractResult, error) {
	return nil, ErrActionServiceMissing
}

func (stubActionService) InvokeAction(context.Context, ActionInvokeRequest) (*ActionInvokeResult, error) {
	return nil, ErrActionServiceMissing
}
