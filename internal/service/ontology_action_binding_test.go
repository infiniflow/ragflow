package service

import (
	"context"
	"errors"
	"testing"

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
