package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// The risk gate is the last thing between "an operator saved a binding" and "an
// agent can change a real system". Both of its inputs default to the safe side,
// so the test that matters is the one where the caller tries to leave that side
// without saying so.
func TestSaveActionBindingRequiresRiskAck(t *testing.T) {
	db := setupServiceTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to access sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&entity.OntologyActionBinding{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	svc := NewActionBindingService()
	ctx := context.Background()

	// Declaring the action writable (read_only=false, the Go zero value) is a
	// risky state: it must be refused before anything is written, and before any
	// attempt to reach an MCP server.
	_, err = svc.SaveBinding(ctx, db, ActionBindingInput{
		TenantID:   "tenant-1",
		TemplateID: "model-1",
		ActionName: "freeze_account",
		MCPServer:  "bank",
		MCPTool:    "freezeCard",
		ReadOnly:   false,
		QACallable: false,
		RiskAck:    false,
	})
	var rejected *ActionEditRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("expected a rejection, got %v", err)
	}
	if rejected.Code != "RISK_NOT_ACKNOWLEDGED" {
		t.Fatalf("expected RISK_NOT_ACKNOWLEDGED, got %s (%s)", rejected.Code, rejected.Message)
	}

	// Nothing was persisted: a refused save must not leave a half-configured row
	// behind for the next reader to find.
	rows, err := svc.bindingDAO.ListByTemplate(ctx, db, "tenant-1", "model-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no binding rows, got %d", len(rows))
	}

	// The same call with the acknowledgement clears the risk gate and fails later,
	// on the server lookup — which is what proves the gate is what stopped it
	// above, rather than an unrelated error.
	_, err = svc.SaveBinding(ctx, db, ActionBindingInput{
		TenantID:   "tenant-1",
		TemplateID: "model-1",
		ActionName: "freeze_account",
		MCPServer:  "bank",
		MCPTool:    "freezeCard",
		ReadOnly:   false,
		RiskAck:    true,
	})
	if !errors.As(err, &rejected) {
		t.Fatalf("expected a rejection, got %v", err)
	}
	if rejected.Code != "MCP_SERVER_UNAVAILABLE" {
		t.Fatalf("expected the call to reach the server lookup, got %s", rejected.Code)
	}
}

// A binding that stays on the safe side needs no acknowledgement: requiring one
// for "not callable, read-only" would train operators to tick the box without
// reading it.
func TestSaveActionBindingSafeCombinationSkipsRiskAck(t *testing.T) {
	db := setupServiceTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to access sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&entity.OntologyActionBinding{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	svc := NewActionBindingService()
	_, err = svc.SaveBinding(context.Background(), db, ActionBindingInput{
		TenantID:   "tenant-1",
		TemplateID: "model-1",
		ActionName: "lookup_balance",
		MCPServer:  "bank",
		MCPTool:    "getBalance",
		ReadOnly:   true,
		QACallable: false,
		RiskAck:    false,
	})
	var rejected *ActionEditRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("expected a rejection, got %v", err)
	}
	if rejected.Code == "RISK_NOT_ACKNOWLEDGED" {
		t.Fatalf("a read-only, non-callable binding must not need an acknowledgement")
	}
}

// The addresses round-trip through the JSON column and come back in the order
// they were saved: the first entry is the default, so order carries meaning.
func TestModelMCPServersPersistThroughJSONColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.CompilationTemplate{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const tenantID = "tenant-1"
	const templateID = "0123456789abcdef0123456789abcdef"
	status := string(entity.StatusValid)
	row := &entity.CompilationTemplate{
		ID:       templateID,
		TenantID: strPointer(tenantID),
		Name:     "model",
		Kind:     "wiki",
		Config:   entity.JSONMap{},
		Status:   &status,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed template: %v", err)
	}

	svc := NewActionBindingService()
	if err := svc.SetModelMCPServers(context.Background(), db, tenantID, templateID, []string{
		" https://b.example/mcp ",
		"https://a.example/mcp",
		"https://b.example/mcp",
		"",
	}); err != nil {
		t.Fatalf("set servers: %v", err)
	}

	got, err := svc.ModelMCPServers(context.Background(), db, tenantID, templateID)
	if err != nil {
		t.Fatalf("read servers: %v", err)
	}
	if len(got) != 2 || got[0] != "https://b.example/mcp" || got[1] != "https://a.example/mcp" {
		t.Fatalf("stored servers = %v, want the two unique addresses in entry order", got)
	}
}

func strPointer(value string) *string { return &value }

// A model may front several upstream systems, so its addresses are a list. The
// list is normalised once here because the picker, the tool cache and the
// executor all read it, and the first entry is the default a binding follows.
func TestModelMCPServersKeepOrderAndDropRepeats(t *testing.T) {
	got := normalizeMCPServers([]string{
		"  https://a.example/mcp  ",
		"",
		"https://b.example/mcp",
		"https://a.example/mcp",
		"   ",
	})
	if len(got) != 2 || got[0] != "https://a.example/mcp" || got[1] != "https://b.example/mcp" {
		t.Fatalf("normalizeMCPServers = %v, want the two trimmed addresses in order", got)
	}

	template := &entity.CompilationTemplate{MCPServers: entity.JSONList(got)}
	if endpoint := modelMCPEndpoint(template); endpoint != "https://a.example/mcp" {
		t.Fatalf("default endpoint = %q, want the first entry", endpoint)
	}
	if endpoint := modelMCPEndpoint(&entity.CompilationTemplate{}); endpoint != "" {
		t.Fatalf("unset model endpoint = %q, want empty", endpoint)
	}
}

// The binding row answers "is this wiring usable", not "may this action run".
// Judging the action gates on a path that never loads an action reported a
// permanent refuse/action_not_found on every row, which is why a correctly bound,
// reachable tool still showed up as refused.
func TestBindingVerdictIsScopedToTheBinding(t *testing.T) {
	verdict, reasons := bindingVerdict(true, true)
	if verdict != "allow" || len(reasons) != 1 || reasons[0] != "allowed" {
		t.Fatalf("usable binding = %q %v, want allow/allowed", verdict, reasons)
	}

	verdict, reasons = bindingVerdict(false, true)
	if verdict != "refuse" || len(reasons) != 1 || reasons[0] != "binding_disabled" {
		t.Fatalf("disabled binding = %q %v, want refuse/binding_disabled", verdict, reasons)
	}

	verdict, reasons = bindingVerdict(true, false)
	if verdict != "refuse" || len(reasons) != 1 || reasons[0] != "tool_unavailable" {
		t.Fatalf("missing tool = %q %v, want refuse/tool_unavailable", verdict, reasons)
	}
}

// A deployment that refuses to dial a private address is not reporting a broken
// server: the endpoint may be healthy and the fix is a deployment setting. The
// two failures therefore carry different codes, and the address survives in the
// message so the reader knows which endpoint was refused.
func TestMCPFetchRejectionSeparatesPrivateHostBlockFromUnavailable(t *testing.T) {
	blocked := fmt.Errorf("tools/list: %w", &common.NonPublicAddressError{Address: "192.168.1.43"})
	rejected := MCPFetchRejection(blocked)
	if rejected.Code != "MCP_PRIVATE_HOST_BLOCKED" {
		t.Fatalf("private-host refusal code = %q, want MCP_PRIVATE_HOST_BLOCKED", rejected.Code)
	}
	if !strings.Contains(rejected.Message, "192.168.1.43") {
		t.Fatalf("message should name the refused address, got %q", rejected.Message)
	}
	if len(rejected.Fields) != 1 || rejected.Fields[0] != "mcp_server" {
		t.Fatalf("the refusal must point at the address field, got %v", rejected.Fields)
	}

	unreachable := MCPFetchRejection(errors.New("dial tcp: connection refused"))
	if unreachable.Code != "MCP_SERVER_UNAVAILABLE" {
		t.Fatalf("plain transport failure code = %q, want MCP_SERVER_UNAVAILABLE", unreachable.Code)
	}
}
