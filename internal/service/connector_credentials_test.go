//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	syncerconnector "ragflow/internal/syncer/connector"
	connectormock "ragflow/internal/syncer/connector/mock"
)

const testConnectorKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

func setupConnectorCredentialsServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	return db
}

func insertConnectorWithToken(t *testing.T, db *gorm.DB, id, token string) {
	t.Helper()
	if err := db.Create(&entity.Connector{
		ID:        id,
		TenantID:  "tenant-1",
		Name:      id,
		Source:    "mock",
		InputType: "poll",
		Config:    entity.ConnectorConfig{"credentials": map[string]interface{}{"api_token": token}, "wiki": "x"},
		Status:    "0",
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
}

func rawConnectorConfigColumn(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var config string
	if err := db.Raw("SELECT config FROM connector WHERE id = ?", id).Scan(&config).Error; err != nil {
		t.Fatalf("read connector config: %v", err)
	}
	return config
}

func connectorToken(config entity.ConnectorConfig) interface{} {
	credentials, _ := config["credentials"].(map[string]interface{})
	return credentials["api_token"]
}

func TestUpdateConnectorEncryptsCredentialsAtRest(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsServiceDB(t)
	insertConnectorWithToken(t, db, "conn-1", "old-token")

	connector, code, err := NewConnectorService().UpdateConnector(t.Context(), "conn-1", "tenant-1", &UpdateConnectorRequest{
		Config: entity.JSONMap{"credentials": map[string]interface{}{"api_token": "tok-123"}, "wiki": "y"},
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateConnector: code=%v err=%v", code, err)
	}
	if connectorToken(connector.Config) != "tok-123" || connector.Config["wiki"] != "y" {
		t.Fatalf("returned config = %#v, want plaintext credentials", connector.Config)
	}
	raw := rawConnectorConfigColumn(t, db, "conn-1")
	if !strings.Contains(raw, `"credentials":"enc:v1:`) || strings.Contains(raw, "tok-123") {
		t.Fatalf("stored config = %s, want encrypted credentials", raw)
	}
}

func TestCreateConnectorEncryptsCredentialsAtRest(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsServiceDB(t)

	connector, err := NewConnectorService().CreateConnector(t.Context(), "tenant-1", &CreateConnectorRequest{
		Name:   "conn",
		Source: "mock",
		Config: entity.JSONMap{"credentials": map[string]interface{}{"api_token": "tok-123"}},
	})
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	if connectorToken(connector.Config) != "tok-123" {
		t.Fatalf("returned config = %#v, want plaintext credentials", connector.Config)
	}
	raw := rawConnectorConfigColumn(t, db, connector.ID)
	if !strings.Contains(raw, `"credentials":"enc:v1:`) || strings.Contains(raw, "tok-123") {
		t.Fatalf("stored config = %s, want encrypted credentials", raw)
	}
}

func TestGetConnectorDecryptsCredentials(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsServiceDB(t)
	insertConnectorWithToken(t, db, "conn-1", "tok-123")
	svc := NewConnectorService()

	connector, err := svc.GetConnector(t.Context(), "conn-1", "tenant-1")
	if err != nil {
		t.Fatalf("GetConnector: %v", err)
	}
	if connectorToken(connector.Config) != "tok-123" {
		t.Fatalf("returned config = %#v, want plaintext credentials", connector.Config)
	}

	t.Setenv(common.EnvRAGFlowConnectorKey, "")
	_, err = svc.GetConnector(t.Context(), "conn-1", "tenant-1")
	if err == nil || err.Error() != "connector credentials are encrypted but RAGFLOW_CONNECTOR_KEY is not set" {
		t.Fatalf("GetConnector without key error = %v", err)
	}
}

func TestConnectorWithLostKeyCanBeUpdatedAndDeleted(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsServiceDB(t)
	insertConnectorWithToken(t, db, "conn-1", "tok-123")
	insertConnectorWithToken(t, db, "conn-2", "tok-456")
	t.Setenv(common.EnvRAGFlowConnectorKey, "")
	svc := NewConnectorService()

	connector, code, err := svc.UpdateConnector(t.Context(), "conn-1", "tenant-1", &UpdateConnectorRequest{
		Config: entity.JSONMap{"credentials": map[string]interface{}{"api_token": "new-token"}},
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateConnector: code=%v err=%v", code, err)
	}
	if connectorToken(connector.Config) != "new-token" {
		t.Fatalf("returned config = %#v, want the new credentials", connector.Config)
	}

	deleted, code, err := svc.DeleteConnector(t.Context(), "conn-2", "tenant-1")
	if err != nil || code != common.CodeSuccess || !deleted {
		t.Fatalf("DeleteConnector: deleted=%v code=%v err=%v", deleted, code, err)
	}
}

func TestUpdateConnectorWithLostKeyAndNoNewConfigChangesNothing(t *testing.T) {
	refreshFreq := int64(30)
	missingKey := "connector credentials are encrypted but RAGFLOW_CONNECTOR_KEY is not set"
	for _, tc := range []struct {
		name string
		req  *UpdateConnectorRequest
		key  string
		err  string
	}{
		{name: "refresh_freq", req: &UpdateConnectorRequest{RefreshFreq: &refreshFreq}, err: missingKey},
		{name: "cancel", req: &UpdateConnectorRequest{Status: "CANCEL"}, err: missingKey},
		{name: "schedule", req: &UpdateConnectorRequest{Status: string(entity.TaskStatusSchedule)}, err: missingKey},
		{name: "reschedule", req: &UpdateConnectorRequest{Reschedule: true}, err: missingKey},
		// Bytes 1..32, not the key the row was written with.
		{name: "wrong key", req: &UpdateConnectorRequest{RefreshFreq: &refreshFreq}, key: "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", err: "cannot decrypt connector credentials: wrong RAGFLOW_CONNECTOR_KEY or corrupted value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
			db := setupConnectorCredentialsServiceDB(t)
			if err := db.AutoMigrate(&entity.Connector2Kb{}, &entity.Knowledgebase{}); err != nil {
				t.Fatalf("migrate kb tables: %v", err)
			}
			insertConnectorWithToken(t, db, "conn-1", "tok-123")
			if err := db.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: "tenant-1", Name: "kb-1", CreatedBy: "tenant-1", EmbdID: "embd"}).Error; err != nil {
				t.Fatalf("insert kb: %v", err)
			}
			if err := db.Create(&entity.Connector2Kb{ID: "conn-1-kb-1", ConnectorID: "conn-1", KbID: "kb-1", AutoParse: "1"}).Error; err != nil {
				t.Fatalf("insert connector2kb: %v", err)
			}
			if err := db.Create(&entity.SyncLogs{
				ID:          "task-1",
				ConnectorID: "conn-1",
				KbID:        "kb-1",
				TaskType:    dao.TaskTypeSync,
				Status:      string(entity.TaskStatusRunning),
			}).Error; err != nil {
				t.Fatalf("insert running task: %v", err)
			}
			t.Setenv(common.EnvRAGFlowConnectorKey, tc.key)

			_, code, err := NewConnectorService().UpdateConnector(t.Context(), "conn-1", "tenant-1", tc.req)
			if err == nil || err.Error() != tc.err || code != common.CodeServerError {
				t.Fatalf("UpdateConnector: code=%v err=%v, want CodeServerError and %q", code, err, tc.err)
			}
			var connector entity.Connector
			if err := db.First(&connector, "id = ?", "conn-1").Error; err != nil {
				t.Fatalf("load connector: %v", err)
			}
			if connector.RefreshFreq != 0 || connector.Status != "0" {
				t.Fatalf("connector refresh_freq/status = %d/%s, want 0/0", connector.RefreshFreq, connector.Status)
			}
			var task entity.SyncLogs
			if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
				t.Fatalf("load task: %v", err)
			}
			if task.Status != string(entity.TaskStatusRunning) {
				t.Fatalf("task status = %s, want running", task.Status)
			}
			var tasks int64
			if err := db.Model(&entity.SyncLogs{}).Count(&tasks).Error; err != nil {
				t.Fatalf("count tasks: %v", err)
			}
			if tasks != 1 {
				t.Fatalf("sync_logs rows = %d, want 1 (no task scheduled)", tasks)
			}
		})
	}
}

func TestUpdateConnectorRejectsEncryptedCredentialsInput(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsServiceDB(t)
	insertConnectorWithToken(t, db, "conn-1", "tok-123")
	before := rawConnectorConfigColumn(t, db, "conn-1")

	_, code, err := NewConnectorService().UpdateConnector(t.Context(), "conn-1", "tenant-1", &UpdateConnectorRequest{
		Config: entity.JSONMap{"credentials": "enc:v1:abc"},
	})
	if !errors.Is(err, ErrConnectorEncryptedCredentials) || code != common.CodeDataError {
		t.Fatalf("UpdateConnector: code=%v err=%v, want CodeDataError and ErrConnectorEncryptedCredentials", code, err)
	}
	if after := rawConnectorConfigColumn(t, db, "conn-1"); after != before {
		t.Fatalf("stored config changed: %s", after)
	}
}

func TestConnectorServiceTestConnectorDecryptsStoredCredentials(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsServiceDB(t)
	insertConnectorWithToken(t, db, "conn-1", "tok-123")

	var capturedConfig map[string]any
	registry := syncerconnector.NewRegistry()
	registry.RegisterConfigFactory("mock", func(config map[string]any) (syncerconnector.Connector, error) {
		capturedConfig = config
		return &connectormock.Connector{}, nil
	})
	svc := NewConnectorService()
	svc.connectorRegistry = registry

	if err := svc.TestConnector(t.Context(), "conn-1", "tenant-1", nil); err != nil {
		t.Fatalf("TestConnector: %v", err)
	}
	if connectorToken(entity.ConnectorConfig(capturedConfig)) != "tok-123" {
		t.Fatalf("config = %#v, want decrypted stored credentials", capturedConfig)
	}

	t.Setenv(common.EnvRAGFlowConnectorKey, "")
	request := entity.JSONMap{"source": "mock", "config": entity.JSONMap{"credentials": map[string]interface{}{"api_token": "new-token"}}}
	if err := svc.TestConnector(t.Context(), "conn-1", "tenant-1", request); err != nil {
		t.Fatalf("TestConnector with request config and no key: %v", err)
	}
	err := svc.TestConnector(t.Context(), "conn-1", "tenant-1", nil)
	if err == nil || !strings.Contains(err.Error(), "RAGFLOW_CONNECTOR_KEY is not set") {
		t.Fatalf("TestConnector with stored config and no key error = %v", err)
	}
}

func TestConnectorServiceTestConnectorRejectsEncryptedCredentialsInput(t *testing.T) {
	setupConnectorCredentialsServiceDB(t)
	registry := syncerconnector.NewRegistry()
	registry.RegisterConfigFactory("mock", func(config map[string]any) (syncerconnector.Connector, error) {
		return &connectormock.Connector{}, nil
	})
	svc := NewConnectorService()
	svc.connectorRegistry = registry

	err := svc.TestConnector(t.Context(), "missing", "tenant-1", entity.JSONMap{
		"source": "mock",
		"config": entity.JSONMap{"credentials": "enc:v1:abc"},
	})
	var valErr *syncerconnector.ConnectorValidationError
	if !errors.As(err, &valErr) || valErr.Message != ErrConnectorEncryptedCredentials.Error() {
		t.Fatalf("error = %v, want *ConnectorValidationError for encrypted input", err)
	}
}
