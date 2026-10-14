package service

import (
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func setupFreqValidationConnector(t *testing.T) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	if err := db.Create(&entity.Connector{
		ID:          "conn-freq",
		TenantID:    "user-1",
		Name:        "conn-freq",
		Source:      "rss",
		InputType:   "poll",
		Config:      entity.JSONMap{},
		Status:      string(entity.TaskStatusCancel),
		RefreshFreq: 5,
		PruneFreq:   5,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
}

func TestCreateConnectorRejectsNonPositiveFreq(t *testing.T) {
	setupFreqValidationConnector(t)
	svc := NewConnectorService()

	for name, req := range map[string]*CreateConnectorRequest{
		"refresh zero":     {Name: "c", Source: "rss", Config: entity.JSONMap{}, RefreshFreq: int64Ptr(0)},
		"refresh negative": {Name: "c", Source: "rss", Config: entity.JSONMap{}, RefreshFreq: int64Ptr(-3)},
		"prune zero":       {Name: "c", Source: "rss", Config: entity.JSONMap{}, PruneFreq: int64Ptr(0)},
		"prune negative":   {Name: "c", Source: "rss", Config: entity.JSONMap{}, PruneFreq: int64Ptr(-1)},
	} {
		if _, err := svc.CreateConnector(t.Context(), "user-1", req); err == nil {
			t.Fatalf("%s: CreateConnector succeeded, want frequency error", name)
		} else if !errors.Is(err, ErrConnectorInvalidFreq) {
			t.Fatalf("%s: error = %v, want ErrConnectorInvalidFreq", name, err)
		}
	}
}

func TestUpdateConnectorRejectsNonPositiveFreq(t *testing.T) {
	setupFreqValidationConnector(t)
	svc := NewConnectorService()

	for name, req := range map[string]*UpdateConnectorRequest{
		"refresh zero":     {RefreshFreq: int64Ptr(0)},
		"refresh negative": {RefreshFreq: int64Ptr(-3)},
		"prune zero":       {PruneFreq: int64Ptr(0)},
		"prune negative":   {PruneFreq: int64Ptr(-1)},
	} {
		_, code, err := svc.UpdateConnector(t.Context(), "conn-freq", "user-1", req)
		if err == nil {
			t.Fatalf("%s: UpdateConnector succeeded, want frequency error", name)
		}
		if code != common.CodeDataError {
			t.Fatalf("%s: code = %v, want CodeDataError", name, code)
		}
	}
}

func int64Ptr(v int64) *int64 { return &v }
