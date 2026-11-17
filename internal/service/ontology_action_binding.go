package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
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

// SetModelMCPServers stores the addresses the model's actions may call.
//
// A model can front more than one upstream system, so the addresses are a list
// rather than a single value: each binding picks one, and the first is what a
// binding follows when it names none. The list is normalised here — trimmed,
// de-duplicated and stripped of blanks — because the picker, the tool cache and
// the executor all read it and must agree on what is in it.
func (s *ActionBindingService) SetModelMCPServers(ctx context.Context, db *gorm.DB, tenantID, templateID string, endpoints []string) error {
	if _, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID); err != nil {
		return err
	}
	servers := normalizeMCPServers(endpoints)
	if err := s.templateDAO.UpdateFields(ctx, db, templateID, map[string]interface{}{
		"mcp_servers": entity.JSONList(servers),
	}); err != nil {
		return err
	}
	// Cached tool listings were keyed by the previous addresses; dropping them
	// stops the next read from reporting tools the model no longer reaches.
	actionToolCache.Range(func(key, _ any) bool {
		actionToolCache.Delete(key)
		return true
	})
	return nil
}

// ModelMCPServers reads the model-level addresses, in the order they were saved.
func (s *ActionBindingService) ModelMCPServers(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]string, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	return normalizeMCPServers([]string(template.MCPServers)), nil
}

// modelDefaultEndpoint is the address a binding follows when it names none.
func (s *ActionBindingService) modelDefaultEndpoint(ctx context.Context, db *gorm.DB, tenantID, templateID string) (string, error) {
	servers, err := s.ModelMCPServers(ctx, db, tenantID, templateID)
	if err != nil || len(servers) == 0 {
		return "", err
	}
	return servers[0], nil
}

// normalizeMCPServers trims, drops blanks and de-duplicates while keeping the
// operator's order: the first entry is the default, so reordering is meaningful.
func normalizeMCPServers(endpoints []string) []string {
	seen := make(map[string]struct{}, len(endpoints))
	out := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		if _, ok := seen[endpoint]; ok {
			continue
		}
		seen[endpoint] = struct{}{}
		out = append(out, endpoint)
	}
	return out
}

// ModelMCPTools lists what one of the model's addresses exposes.
//
// It exists so the run test can offer the tool names instead of asking for one
// from memory: the address is already known, and the only thing the operator
// should have to pick is which tool this action calls. An empty address means
// the model's first entry, which is what a binding with no address of its own
// dials.
func (s *ActionBindingService) ModelMCPTools(ctx context.Context, db *gorm.DB, tenantID, templateID, endpointRef string) ([]mcpclient.Tool, error) {
	endpoint := strings.TrimSpace(endpointRef)
	if endpoint == "" {
		servers, err := s.ModelMCPServers(ctx, db, tenantID, templateID)
		if err != nil {
			return nil, err
		}
		if len(servers) == 0 {
			return nil, nil
		}
		endpoint = servers[0]
	}
	target, err := resolveMCPTarget(ctx, db, tenantID, endpoint)
	if err != nil {
		return nil, err
	}
	return mcpclient.FetchTools(ctx, mcpclient.FetchOptions{
		URL:               target.URL,
		ServerType:        target.ServerType,
		Headers:           target.Headers,
		Variables:         target.Variables,
		Timeout:           10 * time.Second,
		AllowPrivateHosts: server.MCPAllowPrivateHosts(),
	})
}

// effectiveMCPServer is the address a binding actually dials: its own when it
// overrode the model, the model default otherwise.
func effectiveMCPServer(own, modelEndpoint string) string {
	if own = strings.TrimSpace(own); own != "" {
		return own
	}
	return strings.TrimSpace(modelEndpoint)
}

// modelMCPEndpoint reads the default address off a loaded template: the first
// one listed. Empty when the model has none, which is the state it is in until
// someone fills the addresses in.
func modelMCPEndpoint(template *entity.CompilationTemplate) string {
	if template == nil || len(template.MCPServers) == 0 {
		return ""
	}
	return strings.TrimSpace(template.MCPServers[0])
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
	Verdict string `json:"verdict"`
	// EffectiveMCPServer is the address this binding dials after inheritance, so
	// the UI can show what a row with an empty address actually calls.
	EffectiveMCPServer string   `json:"effective_mcp_server,omitempty"`
	VerdictReasons     []string `json:"verdict_reasons,omitempty"`
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
	target, err := resolveMCPTarget(ctx, db, tenantID, serverName)
	if err != nil {
		return nil, err
	}
	tools, err := mcpclient.FetchTools(ctx, mcpclient.FetchOptions{
		URL:               target.URL,
		ServerType:        target.ServerType,
		Headers:           target.Headers,
		Variables:         target.Variables,
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

// MCPFetchRejection explains a failed tool discovery to the caller.
//
// The private-address refusal is reported under its own code because it is not a
// broken server: the deployment's guard refuses to dial that address, so the
// endpoint may be perfectly healthy and the fix is a deployment setting (or a
// publicly reachable endpoint). Folding both into "unavailable" sent readers
// looking for a server that was running the whole time.
func MCPFetchRejection(err error) *ActionEditRejectedError {
	var blocked *common.NonPublicAddressError
	if errors.As(err, &blocked) {
		return &ActionEditRejectedError{
			Code:    "MCP_PRIVATE_HOST_BLOCKED",
			Message: err.Error(),
			Fields:  []string{"mcp_server"},
		}
	}
	return &ActionEditRejectedError{Code: "MCP_SERVER_UNAVAILABLE", Message: err.Error()}
}

// mcpTarget is everything needed to reach an MCP server.
//
// A binding may name a registered server or carry the address itself. The two
// are resolved here rather than at every call site so tools/list, save-time
// validation and execution cannot disagree about what a binding points at.
type mcpTarget struct {
	URL        string
	ServerType string
	Headers    map[string]string
	Variables  map[string]string
	// Registered distinguishes a catalogue entry from an address typed into the
	// binding. Only the former can carry headers and credentials: an ad-hoc
	// address has nowhere to keep them, and sending a secret over cleartext http
	// is refused by the client anyway.
	Registered bool
}

// resolveMCPTarget accepts a registered server id/name or an http(s) address.
//
// The address form exists so an operator can try an endpoint before adding it to
// the tenant's MCP catalogue; it is the same value the run test shows.
func resolveMCPTarget(ctx context.Context, db *gorm.DB, tenantID, ref string) (*mcpTarget, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("mcp server is empty")
	}
	if isMCPAddress(ref) {
		return &mcpTarget{
			URL:        ref,
			ServerType: transportForAddress(ref),
		}, nil
	}
	var servers []entity.MCPServer
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND (id = ? OR name = ?)", tenantID, ref, ref).
		Limit(1).Find(&servers).Error; err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("mcp server %q is not configured for this tenant", ref)
	}
	row := servers[0]
	return &mcpTarget{
		URL:        row.URL,
		ServerType: row.ServerType,
		Headers:    jsonMapStringValues(row.Headers),
		Variables:  jsonMapStringValues(row.Variables),
		Registered: true,
	}, nil
}

func isMCPAddress(ref string) bool {
	lower := strings.ToLower(ref)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// transportForAddress picks the transport for an address that was not registered.
//
// Streamable HTTP is the current transport and the default; the legacy SSE
// endpoint is recognised by its conventional /sse path, which is the only signal
// an address without a server type carries.
func transportForAddress(address string) string {
	parsed, err := url.Parse(address)
	if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/sse") {
		return mcpclient.TransportSSE
	}
	return mcpclient.TransportStreamableHTTP
}

// ListBindings returns every binding of a model, each with its live state.
func (s *ActionBindingService) ListBindings(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]ActionBindingView, error) {
	rows, err := s.bindingDAO.ListByTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	// Read once: a row without its own address follows the model, and this is the
	// only way the list can say what such a row actually calls.
	modelEndpoint, err := s.modelDefaultEndpoint(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	views := make([]ActionBindingView, 0, len(rows))
	for i := range rows {
		views = append(views, s.describeBinding(ctx, db, tenantID, &rows[i], modelEndpoint))
	}
	return views, nil
}

// describeBinding computes tool availability and whether this wiring is usable.
//
// The verdict answers the binding's own question — is the row enabled and is the
// tool on the server — not "may this action run": action existence, publication
// and the contract belong to the action row, which carries its own verdict. This
// path never loads an action, so judging action existence here reported
// "refuse / action_not_found" on every binding row, usable ones included.
func (s *ActionBindingService) describeBinding(ctx context.Context, db *gorm.DB, tenantID string, row *entity.OntologyActionBinding, modelEndpoint string) ActionBindingView {
	view := ActionBindingView{OntologyActionBinding: *row}
	// The row keeps what the operator typed; the address it dials is resolved
	// here so a model-level change is picked up without rewriting every binding.
	server := effectiveMCPServer(row.MCPServer, modelEndpoint)
	view.EffectiveMCPServer = server
	tools, err := s.serverTools(ctx, db, tenantID, server)
	if err != nil {
		view.ToolCheckError = err.Error()
	} else if tool, ok := tools[row.MCPTool]; ok {
		view.ToolAvailable = true
		view.MissingRequiredArgs = missingRequiredArgs(tool, mappingKeys(row.ArgMapping))
	} else {
		view.ToolCheckError = fmt.Sprintf("tool %q is not exposed by server %q", row.MCPTool, server)
	}

	view.Verdict, view.VerdictReasons = bindingVerdict(row.Enabled, view.ToolAvailable)
	return view
}

// bindingVerdict is the binding-scope judgement: the row is switched on and the
// tool it names is on the server it dials. Kept as a pure function so the three
// answers are testable without an MCP server.
func bindingVerdict(enabled, toolAvailable bool) (string, []string) {
	switch {
	case !enabled:
		return string(contract.ContractVerdictRefuse), []string{"binding_disabled"}
	case !toolAvailable:
		return string(contract.ContractVerdictRefuse), []string{"tool_unavailable"}
	default:
		return string(contract.ContractVerdictAllow), []string{"allowed"}
	}
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
	if strings.TrimSpace(in.MCPTool) == "" {
		return nil, &ActionEditRejectedError{Code: ActionEditCodeFieldNotEditable, Message: "mcp_tool is required"}
	}
	// The address may be left to the model, which then contributes its first
	// entry: the model lists the systems its actions reach, and a binding that
	// names none follows that default.
	serverRef := strings.TrimSpace(in.MCPServer)
	if serverRef == "" {
		modelEndpoint, err := s.modelDefaultEndpoint(ctx, db, in.TenantID, in.TemplateID)
		if err != nil {
			return nil, err
		}
		serverRef = modelEndpoint
	}
	if serverRef == "" {
		return nil, &ActionEditRejectedError{
			Code:    ActionEditCodeFieldNotEditable,
			Message: "no MCP address: set the model's address on the actions page, or enter one for this action",
			Fields:  []string{"mcp_server"},
		}
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
	tools, err := s.serverTools(ctx, db, in.TenantID, serverRef)
	if err != nil {
		return nil, MCPFetchRejection(err)
	}
	tool, ok := tools[strings.TrimSpace(in.MCPTool)]
	if !ok {
		return nil, &ActionEditRejectedError{
			Code:    "MCP_TOOL_NOT_FOUND",
			Message: fmt.Sprintf("server %q does not expose tool %q", serverRef, in.MCPTool),
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
	view := s.describeBinding(ctx, db, in.TenantID, row, serverRef)
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
