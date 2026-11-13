package agentic_rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	actionContractToolName = "get_action_contract"
	invokeActionToolName   = "invoke_action"
)

// getActionContractToolDescription states the three things that are easy to get
// wrong, because each of them has an obvious wrong reading:
//
//	missing          is "the document does not say", NOT "no restriction"
//	explicit_none    is the only state that may be reported as "no approval needed"
//	manual provenance may not be quoted or cited, because no chunk says it
//
// They are in the tool description and not only in the system prompt because the
// model reads the description at the moment it decides to call, which is the
// moment the distinction matters.
const getActionContractToolDescription = `Read the execution contract of one action in the selected ontology model: authorization, preconditions, atomic effects, follow-ups, failure policy, and whether audit is required.

Read the returned status before answering:
- "present": the document states this field. Its <quote> is the document's own wording and may be cited.
- "explicit_none": the document says no requirement applies. This is the ONLY state that lets you say "no approval is needed".
- "missing": the document does not state this field. It does NOT mean the action is unrestricted — say the document does not specify it, and never fill the gap with an assumption.
- provenance="manual": an operator entered the value. It is authoritative for execution, but no document says it, so do not quote it or attach a citation to it.

The verdict is computed for you and is not negotiable: "allow" may be described as runnable; "confirm" means a human must approve and you must not claim it was approved; "manual" and "refuse" mean it cannot run — report the reasons, do not re-derive them.

This tool only reads. Use invoke_action to request execution.`

// invokeActionToolDescription makes the call-and-report contract explicit: the
// model proposes, the server decides, and the answer comes back with the reasons.
const invokeActionToolDescription = `Ask the server to run one action through its bound tool.

You do not decide whether it may run. Send the request; the server applies the contract, the binding switches and its own execution switch, and answers one of:
- executed: the tool ran; report what came back.
- confirm: a human has to approve this. Tell the user what needs approving; do not say it happened.
- manual / refuse: it did not run; give the reasons the server returned.

Never claim an action succeeded unless the result says executed. An action that was refused is a fact about the model or the binding, and it is more useful to the user than a fabricated confirmation.

If the action needs an argument the documents cannot supply (an account number, a date), ask the user for it instead of inventing one: an invented identifier sent to a real system is worse than a question.`

// ActionContractTool reads one action's contract.
type ActionContractTool struct{}

// NewActionContractTool returns the contract-reading tool.
func NewActionContractTool() *ActionContractTool { return &ActionContractTool{} }

type actionContractArgs struct {
	Action string `json:"action"`
}

// Info returns the tool metadata.
func (t *ActionContractTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: actionContractToolName,
		Desc: getActionContractToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type: schema.String, Required: true,
				Desc: "The action's name as it appears in the model's assertions.",
			},
		}),
	}, nil
}

// InvokableRun reads the contract.
func (t *ActionContractTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, actionContractToolName, t.invokableRun, argumentsInJSON)
}

func (t *ActionContractTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args actionContractArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("get_action_contract: parse arguments: %w", err)
	}
	if strings.TrimSpace(args.Action) == "" {
		return "", fmt.Errorf("get_action_contract: action must be a non-empty string")
	}
	binding, ok := ontologyBindingFrom(ctx)
	if !ok || !binding.Config.Enabled() {
		return ontologyToolNotice(actionContractToolName, "no_ontology_bound",
			"No ontology model is bound to this conversation, so there is no action to read."), nil
	}
	result, err := GetActionService().ActionContract(ctx, ActionContractRequest{
		UserID:     binding.UserID,
		ModelID:    binding.Config.ModelID,
		DatasetIDs: binding.Config.KnowledgeDatasetIDs,
		ActionName: strings.TrimSpace(args.Action),
	})
	if err != nil {
		return ontologyToolNotice(actionContractToolName, "unavailable", err.Error()), nil
	}
	if result == nil {
		return ontologyToolNotice(actionContractToolName, "action_not_found",
			"No action with that name exists in the model. Check the name against the model's actions instead of guessing a second time."), nil
	}
	return formatActionContract(result), nil
}

// formatActionContract renders the contract as XML.
//
// Every field carries its status and provenance: a value without those two
// attributes is exactly the shape that gets misread as "the document says so".
func formatActionContract(result *ActionContractResult) string {
	var b strings.Builder
	b.WriteString(`<action_contract action="` + xmlEscape(result.ActionName) + `"`)
	b.WriteString(` verdict="` + xmlEscape(result.Verdict) + `"`)
	// The denominator is what this model was asked for, not the fixed six: a
	// narrowed contract that filled both of its fields is complete, and reporting
	// "2/6" would invite the agent to call it incomplete.
	total := len(result.Required)
	if total == 0 {
		total = len(actionContractFieldOrder)
	}
	b.WriteString(fmt.Sprintf(` declared="%d/%d"`, result.Declared, total))
	b.WriteString(fmt.Sprintf(` bound="%t"`, result.Bound))
	if result.Bound {
		b.WriteString(fmt.Sprintf(` binding_enabled="%t" read_only="%t"`, result.BindingEnabled, result.ReadOnly))
	}
	b.WriteString(">\n")

	b.WriteString("  <fields>\n")
	for _, name := range actionContractFieldOrder {
		field, ok := result.Fields[name]
		if !ok {
			continue
		}
		b.WriteString(`    <field name="` + xmlEscape(name) + `" status="` + xmlEscape(field.Status) + `" provenance="` + xmlEscape(field.Provenance) + `">`)
		if field.Status != "missing" && field.Value != nil {
			b.WriteString("<value>" + xmlEscape(renderContractValue(field.Value)) + "</value>")
		}
		if field.Quote != "" {
			b.WriteString("<quote>" + xmlEscape(field.Quote) + "</quote>")
		}
		b.WriteString("</field>\n")
	}
	b.WriteString("  </fields>\n")

	if len(result.Missing) > 0 {
		b.WriteString(`  <missing_fields note="the document does not specify these; this is not permission">` +
			xmlEscape(strings.Join(result.Missing, ",")) + "</missing_fields>\n")
	}
	if len(result.Reasons) > 0 {
		b.WriteString(`  <verdict_reasons>` + xmlEscape(strings.Join(result.Reasons, ",")) + "</verdict_reasons>\n")
	}
	switch result.Verdict {
	case "allow":
		b.WriteString("  <notice>runnable: the contract places no approval requirement on this action</notice>\n")
	case "confirm":
		b.WriteString("  <notice>a human must approve before it runs; do not report it as approved or done</notice>\n")
	case "manual":
		b.WriteString("  <notice>it cannot run automatically; relay the reasons instead of working around them</notice>\n")
	case "refuse":
		b.WriteString("  <notice>it is refused outright; the reasons above say why</notice>\n")
	}
	b.WriteString("</action_contract>")
	return b.String()
}

// actionContractFieldOrder is the display order, matching the API.
var actionContractFieldOrder = []string{
	"authorization", "preconditions", "atomic_effects", "follow_ups", "failure_policy", "audit_required",
}

func renderContractValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, "; ")
	case []string:
		return strings.Join(v, "; ")
	}
	return fmt.Sprint(value)
}

// InvokeActionTool asks the server to run an action.
type InvokeActionTool struct{}

// NewInvokeActionTool returns the execution tool.
func NewInvokeActionTool() *InvokeActionTool { return &InvokeActionTool{} }

type invokeActionArgs struct {
	Action    string         `json:"action"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// Info returns the tool metadata.
func (t *InvokeActionTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: invokeActionToolName,
		Desc: invokeActionToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type: schema.String, Required: true,
				Desc: "The action's name as it appears in the model's assertions.",
			},
			"arguments": {
				Type: schema.Object,
				Desc: "Optional values the user supplied that the documents cannot " +
					"(an account number, a date). Omit to use the contract and attributes alone.",
			},
		}),
	}, nil
}

// InvokableRun requests execution.
func (t *InvokeActionTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, invokeActionToolName, t.invokableRun, argumentsInJSON)
}

func (t *InvokeActionTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args invokeActionArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invoke_action: parse arguments: %w", err)
	}
	if strings.TrimSpace(args.Action) == "" {
		return "", fmt.Errorf("invoke_action: action must be a non-empty string")
	}
	binding, ok := ontologyBindingFrom(ctx)
	if !ok || !binding.Config.Enabled() {
		return ontologyToolNotice(invokeActionToolName, "no_ontology_bound",
			"No ontology model is bound to this conversation, so there is no action to run."), nil
	}
	result, err := GetActionService().InvokeAction(ctx, ActionInvokeRequest{
		UserID:     binding.UserID,
		ModelID:    binding.Config.ModelID,
		DatasetIDs: binding.Config.KnowledgeDatasetIDs,
		ActionName: strings.TrimSpace(args.Action),
		Arguments:  args.Arguments,
	})
	if err != nil {
		// A refusal is a legitimate answer about the world, not a tool failure: it
		// must reach the model as content it can relay.
		return ontologyToolNotice(invokeActionToolName, "not_executed", err.Error()), nil
	}
	if result == nil {
		return ontologyToolNotice(invokeActionToolName, "not_executed", "the action service returned nothing"), nil
	}
	return formatActionInvocation(result), nil
}

func formatActionInvocation(result *ActionInvokeResult) string {
	var b strings.Builder
	b.WriteString(`<action_invocation verdict="` + xmlEscape(result.Verdict) + `"`)
	b.WriteString(fmt.Sprintf(` executed="%t"`, result.Executed))
	if result.Status != "" {
		b.WriteString(` status="` + xmlEscape(result.Status) + `"`)
	}
	b.WriteString(fmt.Sprintf(` duration_ms="%d">`, result.DurationMS))
	if len(result.Reasons) > 0 {
		b.WriteString("<reasons>" + xmlEscape(strings.Join(result.Reasons, ",")) + "</reasons>")
	}
	if result.Output != "" {
		b.WriteString("<output>" + xmlEscape(result.Output) + "</output>")
	}
	if result.Error != "" {
		b.WriteString("<error>" + xmlEscape(result.Error) + "</error>")
	}
	if !result.Executed {
		b.WriteString("<notice>nothing ran. Report this outcome as it is.</notice>")
	}
	b.WriteString("</action_invocation>")
	return b.String()
}

var (
	_ einotool.InvokableTool = (*ActionContractTool)(nil)
	_ einotool.InvokableTool = (*InvokeActionTool)(nil)
)
