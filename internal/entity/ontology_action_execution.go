package entity

// OntologyActionExecution is the audit record of one attempt to run an action.
//
// Refusals are recorded too, not only successes. The interesting question after
// an incident is "what did the agent try", and a log that only keeps successful
// calls answers the opposite one.
//
// The verdict is stored, not recomputed: it is the judgement that was in force at
// the time. If an operator later tightens a contract, the history must still show
// what the rules said when the call happened.
type OntologyActionExecution struct {
	ID         string `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID   string `gorm:"column:tenant_id;size:32;not null;index:idx_action_exec_scope" json:"tenant_id"`
	TemplateID string `gorm:"column:template_id;size:32;not null;default:'';index:idx_action_exec_scope" json:"template_id"`
	ActionName string `gorm:"column:action_name;size:255;not null;default:'';index:idx_action_exec_scope" json:"action_name"`
	BindingID  string `gorm:"column:binding_id;size:32;not null;default:''" json:"binding_id"`
	// Mode is "query" (the agent called it during a turn) or "manual" (an operator
	// pressed run). The two have different verdict rules, so the row must say which.
	Mode       string   `gorm:"column:mode;size:16;not null;default:''" json:"mode"`
	MCPTool    string   `gorm:"column:mcp_tool;size:255;not null;default:''" json:"mcp_tool"`
	Verdict    string   `gorm:"column:verdict;size:16;not null;default:''" json:"verdict"`
	Status     string   `gorm:"column:status;size:16;not null;default:''" json:"status"`
	RequestArg *JSONMap `gorm:"column:request_arg;type:json" json:"request_arg,omitempty"`
	// ResponseExcerpt is truncated: the audit needs to show what came back, not to
	// become a second copy of the upstream system's data.
	ResponseExcerpt string `gorm:"column:response_excerpt;type:text" json:"response_excerpt"`
	Error           string `gorm:"column:error;type:text" json:"error"`
	DurationMS      int    `gorm:"column:duration_ms;not null;default:0" json:"duration_ms"`
	OperatorID      string `gorm:"column:operator_id;size:32;not null;default:''" json:"operator_id"`
	ConfirmedBy     string `gorm:"column:confirmed_by;size:32;not null;default:''" json:"confirmed_by"`
	BaseModel
}

// TableName returns the table name for OntologyActionExecution.
func (OntologyActionExecution) TableName() string {
	return "ontology_action_execution"
}

// Execution modes.
const (
	ActionExecutionModeQuery  = "query"
	ActionExecutionModeManual = "manual"
)

// Execution statuses.
const (
	ActionExecutionStatusSucceeded = "succeeded"
	ActionExecutionStatusFailed    = "failed"
	ActionExecutionStatusRefused   = "refused"
	ActionExecutionStatusTimeout   = "timeout"
)
