package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	mcpclient "ragflow/internal/mcp/client"
	"ragflow/internal/ontology/contract"
	"ragflow/internal/server"

	"gorm.io/gorm"
)

// Action bindings: which MCP tool executes a compiled action, and under which
// switches.
//
// The mapping is deliberately an explicit, inspectable table rather than a
// convention over names. "冻结账户" running `bank.freezeCard` is a business
// decision that needs an owner; guessing it from a naming pattern is how an agent
// ends up calling the wrong tool with the right-looking arguments.

// ActionArgExpression prefixes for arg_mapping values. They are a tiny, closed
// language on purpose: the alternative (letting the mapping run code or query the
// engine) makes the binding a program that nobody can review, and the review is
// the point.
const (
	ArgExpressionContract = "contract."
	ArgExpressionAction   = "action."
	ArgExpressionAttr     = "attr."
	ArgExpressionLiteral  = "literal:"
)

// ActionExecEnabled reports the process-wide execution switch.
//
// Default off, and read per call rather than captured at startup: an operator
// turning execution off during an incident must not wait for a restart. It sits
// below the per-binding switches — a binding can never turn execution on by
// itself.
func ActionExecEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RAGFLOW_ONTOLOGY_ACTION_EXEC"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ActionBindingInput is one binding write.
type ActionBindingInput struct {
	TenantID   string
	TemplateID string
	BindingID  string
	ActionName string
	DatasetID  string
	MCPServer  string
	MCPTool    string
	ArgMapping map[string]string
	ReadOnly   bool
	Enabled    bool
	QACallable bool
	// RiskAck records that the operator read what enabling an unattended call
	// means. It is required for exactly the two switches that create risk —
	// being callable by an agent, and not being declared read-only — and ignored
	// otherwise, so "I accept" cannot be pre-ticked into a safe binding.
	RiskAck bool
	Note    string
	ActorID string
}

// ActionBindingView is a binding plus everything the workbench needs to explain
// why it is or is not executable right now.
type ActionBindingView struct {
	entity.OntologyActionBinding
	// ToolAvailable is false when the MCP server cannot be reached or no longer
	// exposes the tool. Shown as its own state because it is the one failure an
	// operator cannot fix on this page.
	ToolAvailable bool `json:"tool_available"`
	// ToolCheckError carries the reason, so "unavailable" is actionable.
	ToolCheckError string `json:"tool_check_error,omitempty"`
	// MissingRequiredArgs lists tool parameters the mapping does not supply. A
	// binding in this state would fail at call time with an upstream error; it must
	// be refused before the call instead.
	MissingRequiredArgs []string `json:"missing_required_args,omitempty"`
	// Verdict is the gate result as of now, and Reasons explains it. Computed for
	// every row so the list and the detail panel cannot disagree.
	Verdict        string   `json:"verdict"`
	VerdictReasons []string `json:"verdict_reasons,omitempty"`
}

// ActionBindingService owns bindings and answers gate questions about them.
type ActionBindingService struct {
	bindingDAO  *dao.OntologyActionBindingDAO
	execDAO     *dao.OntologyActionExecutionDAO
	templateDAO *dao.CompilationTemplateDAO
}

// NewActionBindingService creates the binding service.
func NewActionBindingService() *ActionBindingService {
	return &ActionBindingService{
		bindingDAO:  dao.NewOntologyActionBindingDAO(),
		execDAO:     dao.NewOntologyActionExecutionDAO(),
		templateDAO: dao.NewCompilationTemplateDAO(),
	}
}

// toolCache remembers tools listed per server so a list page makes one call per
// server instead of one per binding. Availability is recomputed on expiry because
// a tool can disappear without any write to this table.
type toolCacheEntry struct {
	tools   map[string]mcpclient.Tool
	err     error
	fetched time.Time
}

var actionToolCache sync.Map

const actionToolCacheTTL = 60 * time.Second

func (s *ActionBindingService) serverTools(ctx context.Context, db *gorm.DB, tenantID, serverName string) (map[string]mcpclient.Tool, error) {
	key := tenantID + "\x00" + serverName
	if cached, ok := actionToolCache.Load(key); ok {
		entry := cached.(toolCacheEntry)
		if time.Since(entry.fetched) < actionToolCacheTTL {
			return entry.tools, entry.err
		}
	}
	tools, err := s.fetchServerTools(ctx, db, tenantID, serverName)
	actionToolCache.Store(key, toolCacheEntry{tools: tools, err: err, fetched: time.Now()})
	return tools, err
}

func (s *ActionBindingService) fetchServerTools(ctx context.Context, db *gorm.DB, tenantID, serverName string) (map[string]mcpclient.Tool, error) {
	var servers []entity.MCPServer
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND (id = ? OR name = ?)", tenantID, serverName, serverName).
		Limit(1).Find(&servers).Error; err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("mcp server %q is not configured for this tenant", serverName)
	}
	row := servers[0]
	tools, err := mcpclient.FetchTools(ctx, mcpclient.FetchOptions{
		URL:               row.URL,
		ServerType:        row.ServerType,
		Headers:           jsonMapStringValues(row.Headers),
		Variables:         jsonMapStringValues(row.Variables),
		Timeout:           10 * time.Second,
		AllowPrivateHosts: server.MCPAllowPrivateHosts(),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]mcpclient.Tool, len(tools))
	for _, tool := range tools {
		out[tool.Name] = tool
	}
	return out, nil
}

// ListBindings returns every binding of a model, each with its live state.
func (s *ActionBindingService) ListBindings(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]ActionBindingView, error) {
	rows, err := s.bindingDAO.ListByTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	views := make([]ActionBindingView, 0, len(rows))
	for i := range rows {
		views = append(views, s.describeBinding(ctx, db, tenantID, &rows[i], nil))
	}
	return views, nil
}

// describeBinding computes tool availability and the gate verdict for one row.
//
// action carries the compiled action's contract when the caller already has it
// (the agent path does), which keeps this from re-querying the engine per row.
func (s *ActionBindingService) describeBinding(ctx context.Context, db *gorm.DB, tenantID string, row *entity.OntologyActionBinding, action *compiledAction) ActionBindingView {
	view := ActionBindingView{OntologyActionBinding: *row}
	tools, err := s.serverTools(ctx, db, tenantID, row.MCPServer)
	if err != nil {
		view.ToolCheckError = err.Error()
	} else if tool, ok := tools[row.MCPTool]; ok {
		view.ToolAvailable = true
		view.MissingRequiredArgs = missingRequiredArgs(tool, mappingKeys(row.ArgMapping))
	} else {
		view.ToolCheckError = fmt.Sprintf("tool %q is not exposed by server %q", row.MCPTool, row.MCPServer)
	}

	gate := contract.ActionGateInput{
		ActionExists:      action != nil,
		TemplatePublished: true,
		Bound:             true,
		BindingEnabled:    row.Enabled,
		ToolAvailable:     view.ToolAvailable,
		QACallable:        row.QACallable,
		ReadOnly:          row.ReadOnly,
		ExecEnabled:       ActionExecEnabled(),
		Contract:          contract.NewActionContract(),
	}
	if action != nil {
		gate.Contract = action.Contract
	}
	result := contract.EvaluateActionGates(gate)
	view.Verdict = string(result.Verdict)
	view.VerdictReasons = result.Reasons
	return view
}

// missingRequiredArgs compares a tool's required parameters against the mapping.
func missingRequiredArgs(tool mcpclient.Tool, mapped []string) []string {
	required := stringSliceValue(tool.InputSchema["required"])
	if len(required) == 0 {
		return nil
	}
	have := make(map[string]bool, len(mapped))
	for _, key := range mapped {
		have[key] = true
	}
	missing := make([]string, 0, len(required))
	for _, name := range required {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func mappingKeys(mapping *entity.JSONMap) []string {
	if mapping == nil {
		return nil
	}
	out := make([]string, 0, len(*mapping))
	for key := range *mapping {
		out = append(out, key)
	}
	return out
}

// SaveBinding validates and upserts one binding.
//
// Validation is not a formality: a binding that names a missing tool, or that
// leaves a required tool parameter unmapped, fails at call time with an upstream
// error that looks like the action failed rather than like the wiring is wrong.
func (s *ActionBindingService) SaveBinding(ctx context.Context, db *gorm.DB, in ActionBindingInput) (*ActionBindingView, error) {
	actionName := strings.TrimSpace(in.ActionName)
	if actionName == "" {
		return nil, &ActionEditRejectedError{Code: ActionEditCodeActionNotFound, Message: "action name is required"}
	}
	if strings.TrimSpace(in.MCPServer) == "" || strings.TrimSpace(in.MCPTool) == "" {
		return nil, &ActionEditRejectedError{Code: ActionEditCodeFieldNotEditable, Message: "mcp_server and mcp_tool are required"}
	}
	// The risk gate. Both switches default to the safe side (not callable, not
	// declared writable), so reaching either of the two risky states is a decision
	// someone has to make explicitly rather than a checkbox they scrolled past.
	if (in.QACallable || !in.ReadOnly) && !in.RiskAck {
		return nil, &ActionEditRejectedError{
			Code:    "RISK_NOT_ACKNOWLEDGED",
			Message: "allowing an agent to call this action, or declaring it writable, requires an explicit acknowledgement",
			Fields:  []string{"qa_callable", "read_only"},
		}
	}
	tools, err := s.serverTools(ctx, db, in.TenantID, strings.TrimSpace(in.MCPServer))
	if err != nil {
		return nil, &ActionEditRejectedError{Code: "MCP_SERVER_UNAVAILABLE", Message: err.Error()}
	}
	tool, ok := tools[strings.TrimSpace(in.MCPTool)]
	if !ok {
		return nil, &ActionEditRejectedError{
			Code:    "MCP_TOOL_NOT_FOUND",
			Message: fmt.Sprintf("server %q does not expose tool %q", in.MCPServer, in.MCPTool),
		}
	}
	mapped := make([]string, 0, len(in.ArgMapping))
	for key := range in.ArgMapping {
		mapped = append(mapped, key)
	}
	if missing := missingRequiredArgs(tool, mapped); len(missing) > 0 {
		return nil, &ActionEditRejectedError{
			Code:    "MCP_ARGS_UNMAPPED",
			Message: "every required tool parameter must be mapped; an unmapped one fails at call time",
			Fields:  missing,
		}
	}
	for arg, expression := range in.ArgMapping {
		if err := validateArgExpression(expression); err != nil {
			return nil, &ActionEditRejectedError{
				Code:    ActionEditCodeFieldNotEditable,
				Message: fmt.Sprintf("argument %q: %s", arg, err.Error()),
				Fields:  []string{arg},
			}
		}
	}

	mapping := entity.JSONMap{}
	for key, value := range in.ArgMapping {
		mapping[key] = value
	}
	row := &entity.OntologyActionBinding{
		ID:         strings.TrimSpace(in.BindingID),
		TenantID:   in.TenantID,
		TemplateID: in.TemplateID,
		ActionName: actionName,
		DatasetID:  strings.TrimSpace(in.DatasetID),
		MCPServer:  strings.TrimSpace(in.MCPServer),
		MCPTool:    strings.TrimSpace(in.MCPTool),
		ArgMapping: &mapping,
		ReadOnly:   in.ReadOnly,
		Enabled:    in.Enabled,
		QACallable: in.QACallable,
		Note:       strings.TrimSpace(in.Note),
		CreatedBy:  in.ActorID,
	}
	if row.ID == "" {
		// One binding per action: an existing row is updated rather than duplicated,
		// so the unique index never has to reject a legitimate save.
		existing, err := s.bindingDAO.FindByAction(ctx, db, in.TenantID, in.TemplateID, actionName)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			row.ID = existing.ID
		} else {
			row.ID = newOntologyUUID()
		}
	}
	if err := s.bindingDAO.Save(ctx, db, row); err != nil {
		return nil, err
	}
	view := s.describeBinding(ctx, db, in.TenantID, row, nil)
	return &view, nil
}

// validateArgExpression rejects an expression the executor cannot resolve.
//
// Checking it at save time is the difference between "the binding is wrong" shown
// next to the field and "the tool call failed" shown after an incident.
func validateArgExpression(expression string) error {
	expression = strings.TrimSpace(expression)
	switch {
	case strings.HasPrefix(expression, ArgExpressionContract):
		field := strings.TrimPrefix(expression, ArgExpressionContract)
		for _, name := range contract.ActionContractFields() {
			if name == field {
				return nil
			}
		}
		return fmt.Errorf("unknown contract field %q", field)
	case strings.HasPrefix(expression, ArgExpressionAction):
		switch strings.TrimPrefix(expression, ArgExpressionAction) {
		case "name", "description", "type":
			return nil
		}
		return fmt.Errorf("action expression must be name, description or type")
	case strings.HasPrefix(expression, ArgExpressionAttr):
		if strings.TrimPrefix(expression, ArgExpressionAttr) == "" {
			return fmt.Errorf("attribute name is empty")
		}
		return nil
	case strings.HasPrefix(expression, ArgExpressionLiteral):
		return nil
	}
	return fmt.Errorf("expression must start with %q, %q, %q or %q",
		ArgExpressionContract, ArgExpressionAction, ArgExpressionAttr, ArgExpressionLiteral)
}

// DeleteBinding removes one binding.
func (s *ActionBindingService) DeleteBinding(ctx context.Context, db *gorm.DB, tenantID, bindingID string) error {
	if strings.TrimSpace(bindingID) == "" {
		return &ActionEditRejectedError{Code: "BINDING_NOT_FOUND", Message: "empty binding id"}
	}
	return s.bindingDAO.Delete(ctx, db, tenantID, bindingID)
}

// ExecutionQuery is one page of the audit trail plus the filters that chose it.
type ExecutionQuery struct {
	ActionName string
	Verdict    string
	Status     string
	Mode       string
	// FromMS/ToMS bound create_time, unix milliseconds. Zero means unbounded.
	FromMS   int64
	ToMS     int64
	Page     int
	PageSize int
}

// ListExecutions returns the audit trail of a model.
//
// The filters travel to SQL: narrowing a page client-side shows the decisions
// inside the loaded window, which reads as "this is how often it was refused"
// while the answer is "how often it was refused among the last fifty rows".
func (s *ActionBindingService) ListExecutions(
	ctx context.Context, db *gorm.DB, tenantID, templateID string, in ExecutionQuery,
) ([]entity.OntologyActionExecution, int64, error) {
	page := in.Page
	if page < 1 {
		page = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	return s.execDAO.ListByTemplate(ctx, db, tenantID, templateID, dao.ExecutionFilter{
		ActionName: strings.TrimSpace(in.ActionName),
		Verdict:    strings.TrimSpace(in.Verdict),
		Status:     strings.TrimSpace(in.Status),
		Mode:       strings.TrimSpace(in.Mode),
		FromMS:     in.FromMS,
		ToMS:       in.ToMS,
		Offset:     (page - 1) * pageSize,
		Limit:      pageSize,
	})
}
