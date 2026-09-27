package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
	syncerconnector "ragflow/internal/syncer/connector"
	connectormock "ragflow/internal/syncer/connector/mock"
)

func TestConnectorServiceTestConnectorUsesRequestConfig(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	if err := db.Create(&entity.Connector{
		ID:        "conn-1",
		TenantID:  "tenant-1",
		Name:      "conn-1",
		Source:    "mock",
		InputType: "poll",
		Config:    entity.JSONMap{"from": "stored"},
		Status:    "0",
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}

	var capturedConfig map[string]any
	registry := syncerconnector.NewRegistry()
	registry.RegisterConfigFactory("mock", func(config map[string]any) (syncerconnector.Connector, error) {
		capturedConfig = config
		return &connectormock.Connector{}, nil
	})
	svc := NewConnectorService()
	svc.connectorRegistry = registry

	request := entity.JSONMap{
		"source": "mock",
		"config": entity.JSONMap{"from": "request"},
	}
	if err := svc.TestConnector(t.Context(), "conn-1", "tenant-1", request); err != nil {
		t.Fatalf("TestConnector failed: %v", err)
	}
	if capturedConfig["from"] != "request" {
		t.Fatalf("config = %v, want request config", capturedConfig)
	}
}

func TestConnectorServiceTestConnectorAllowsUnsavedConnectorWithSource(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	registry := syncerconnector.NewRegistry()
	registry.RegisterConfigFactory("mock", func(config map[string]any) (syncerconnector.Connector, error) {
		return &connectormock.Connector{}, nil
	})
	svc := NewConnectorService()
	svc.connectorRegistry = registry

	err := svc.TestConnector(t.Context(), "missing", "tenant-1", entity.JSONMap{
		"source": "mock",
		"config": entity.JSONMap{"ok": true},
	})
	if err != nil {
		t.Fatalf("TestConnector failed: %v", err)
	}
}

func TestConnectorServiceTestConnectorSurfacesRawValidationError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	registry := syncerconnector.NewRegistry()
	registry.RegisterConfigFactory("mock", func(config map[string]any) (syncerconnector.Connector, error) {
		return &connectormock.Connector{ValidateConnectorSettingErr: errors.New("raw validation failure")}, nil
	})
	svc := NewConnectorService()
	svc.connectorRegistry = registry

	err := svc.TestConnector(t.Context(), "missing", "tenant-1", entity.JSONMap{
		"source": "mock",
		"config": entity.JSONMap{"ok": true},
	})
	var valErr *syncerconnector.ConnectorValidationError
	if !errors.As(err, &valErr) || !strings.Contains(valErr.Message, "raw validation failure") {
		t.Fatalf("error = %v, want *ConnectorValidationError with raw message", err)
	}
}

func TestConnectorServiceTestConnectorRejectsMissingConfigForMissingConnector(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	err := NewConnectorService().TestConnector(t.Context(), "missing", "tenant-1", nil)
	if !errors.Is(err, ErrConnectorNotFound) {
		t.Fatalf("error = %v, want ErrConnectorNotFound", err)
	}
}

func TestConnectorServiceTestConnectorRejectsUnauthorizedConnector(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	if err := db.Create(&entity.Connector{
		ID:        "conn-1",
		TenantID:  "tenant-1",
		Name:      "conn-1",
		Source:    "mock",
		InputType: "poll",
		Config:    entity.JSONMap{"ok": true},
		Status:    "0",
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}

	err := NewConnectorService().TestConnector(t.Context(), "conn-1", "user-2", entity.JSONMap{
		"source": "mock",
		"config": entity.JSONMap{"ok": true},
	})
	if !errors.Is(err, ErrConnectorNoAuth) {
		t.Fatalf("error = %v, want ErrConnectorNoAuth", err)
	}
}

func TestConnectorServiceTestConnectorRejectsUnsupportedSource(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	err := NewConnectorService().TestConnector(t.Context(), "missing", "tenant-1", entity.JSONMap{
		"source": "unknown",
		"config": entity.JSONMap{"ok": true},
	})
	if !errors.Is(err, ErrConnectorSourceNotImplemented) || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %v, want source not implemented", err)
	}
}

func TestConnectorServiceTestConnectorRejectsConnectorWithoutValidator(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	registry := syncerconnector.NewRegistry()
	registry.RegisterConfigFactory("plain", func(config map[string]any) (syncerconnector.Connector, error) {
		return plainConnector{}, nil
	})
	svc := NewConnectorService()
	svc.connectorRegistry = registry

	err := svc.TestConnector(t.Context(), "missing", "tenant-1", entity.JSONMap{
		"source": "plain",
		"config": entity.JSONMap{"ok": true},
	})
	if !errors.Is(err, ErrConnectorTestUnsupported) {
		t.Fatalf("error = %v, want ErrConnectorTestUnsupported", err)
	}
}

type plainConnector struct{}

func (plainConnector) Validate(context.Context) error { return nil }

func (plainConnector) OpenSync(context.Context, syncerconnector.SyncRequest) (syncerconnector.SyncSession, error) {
	return nil, nil
}

func (plainConnector) OpenPrune(context.Context, syncerconnector.PruneRequest) (syncerconnector.PruneSession, error) {
	return nil, nil
}
