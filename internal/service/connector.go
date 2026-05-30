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
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
)

const (
	connectorInputTypePoll   = "poll"
	connectorStatusUnstarted = "0"
	defaultConnectorFreq     = 5
	defaultConnectorTimeout  = 60 * 29
)

// Sentinel errors so handlers can map to the proper response codes,
// mirroring the Python connector_api responses.
var (
	// ErrConnectorNotFound mirrors Python's "Can't find this Connector!".
	ErrConnectorNotFound = errors.New("can't find this Connector")
	// ErrConnectorNoAuth mirrors Python's "No authorization." denial.
	ErrConnectorNoAuth = errors.New("no authorization")
	// ErrConnectorTestUnsupported is returned for connector sources whose
	// validation path is not yet ported to Go.
	ErrConnectorTestUnsupported = errors.New("test endpoint currently supports only REST API connectors")
)

// ConnectorService connector service
type ConnectorService struct {
	connectorDAO  *dao.ConnectorDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewConnectorService create connector service
func NewConnectorService() *ConnectorService {
	return &ConnectorService{
		connectorDAO:  dao.NewConnectorDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// ListConnectorsResponse list connectors response
type ListConnectorsResponse struct {
	Connectors []*dao.ConnectorListItem `json:"connectors"`
}

// CreateConnectorRequest creates a connector with Python-compatible defaults.
type CreateConnectorRequest struct {
	Name        string         `json:"name"`
	Source      string         `json:"source"`
	Config      entity.JSONMap `json:"config"`
	RefreshFreq *int64         `json:"refresh_freq,omitempty"`
	PruneFreq   *int64         `json:"prune_freq,omitempty"`
	TimeoutSecs *int64         `json:"timeout_secs,omitempty"`
}

// RebuildConnectorRequest rebuild connector request.
type RebuildConnectorRequest struct {
	KbID string `json:"kb_id"`
}

// canAccessConnector Test Authentication
func (s *ConnectorService) canAccessConnector(connector *entity.Connector, userID string) bool {
	if connector.TenantID == userID {
		return true
	}

	_, err := s.userTenantDAO.FilterByUserIDAndTenantID(userID, connector.TenantID)
	return err == nil
}

// cancelConnectorTasks Stop connector tasks
func (s *ConnectorService) cancelConnectorTasks(connectorID string) error {
	if err := s.connectorDAO.CancelRunningOrScheduledLogs(connectorID); err != nil {
		return err
	}
	return s.connectorDAO.UpdateByID(connectorID, map[string]interface{}{"status": string(entity.TaskStatusCancel)})
}

// CreateConnector creates a connector owned by the current user.
// Equivalent to Python's create_connector endpoint.
func (s *ConnectorService) CreateConnector(userID string, req *CreateConnectorRequest) (*entity.Connector, error) {
	refreshFreq := int64(defaultConnectorFreq)
	if req.RefreshFreq != nil {
		refreshFreq = *req.RefreshFreq
	}

	pruneFreq := int64(defaultConnectorFreq)
	if req.PruneFreq != nil {
		pruneFreq = *req.PruneFreq
	}

	timeoutSecs := int64(defaultConnectorTimeout)
	if req.TimeoutSecs != nil {
		timeoutSecs = *req.TimeoutSecs
	}

	connector := &entity.Connector{
		ID:          common.GenerateUUID(),
		TenantID:    userID,
		Name:        req.Name,
		Source:      req.Source,
		InputType:   connectorInputTypePoll,
		Config:      req.Config,
		RefreshFreq: refreshFreq,
		PruneFreq:   pruneFreq,
		TimeoutSecs: timeoutSecs,
		Status:      connectorStatusUnstarted,
	}

	if err := s.connectorDAO.Create(connector); err != nil {
		return nil, err
	}

	return s.connectorDAO.GetByID(connector.ID)
}

// GetConnector returns one connector when the user can access its tenant.
func (s *ConnectorService) GetConnector(connectorID, userID string) (*entity.Connector, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return nil, common.CodeServerError, err
	}

	if connector.TenantID == userID {
		return connector, common.CodeSuccess, nil
	}

	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	for _, tenantID := range tenantIDs {
		if tenantID == connector.TenantID {
			return connector, common.CodeSuccess, nil
		}
	}

	return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
}

// ListConnectors list connectors for a user
// Equivalent to Python's ConnectorService.list(current_user.id)
func (s *ConnectorService) ListConnectors(userID string) (*ListConnectorsResponse, error) {
	// Get tenant IDs by user ID
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	// For now, use the first tenant ID (primary tenant)
	// This matches the Python implementation behavior
	var tenantID string
	if len(tenantIDs) > 0 {
		tenantID = tenantIDs[0]
	} else {
		tenantID = userID
	}

	// Query connectors by tenant ID
	connectors, err := s.connectorDAO.ListByTenantID(tenantID)
	if err != nil {
		return nil, err
	}

	return &ListConnectorsResponse{
		Connectors: connectors,
	}, nil
}

// accessible reports whether the user can access the connector's tenant.
// Mirrors Python's ConnectorService.accessible: owner access plus joined tenants.
func (s *ConnectorService) accessible(connectorID, userID string) (bool, error) {
	conn, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return false, ErrConnectorNotFound
	}

	if conn.TenantID == userID {
		return true, nil
	}

	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return false, err
	}
	for _, tid := range tenantIDs {
		if tid == conn.TenantID {
			return true, nil
		}
	}
	return false, nil
}

// TestConnector validates a connector's stored configuration.
// Equivalent to Python's test_connector. Per-connector credential validation
// lives in the Python common.data_source package and is not yet available in
// Go; for now this verifies access, that the connector exists, that the source
// is REST_API (the only source Python currently tests), and that credentials
// are present in the stored config. It returns ErrConnectorTestUnsupported for
// other sources.
func (s *ConnectorService) TestConnector(connectorID, userID string) error {
	ok, err := s.accessible(connectorID, userID)
	if err != nil && errors.Is(err, ErrConnectorNotFound) {
		return ErrConnectorNotFound
	}
	if err != nil {
		return err
	}
	if !ok {
		return ErrConnectorNoAuth
	}

	conn, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return ErrConnectorNotFound
	}

	if conn.Source != "rest_api" {
		return ErrConnectorTestUnsupported
	}

	config := conn.Config
	if config == nil {
		return fmt.Errorf("connector configuration is missing")
	}
	creds, ok := config["credentials"].(map[string]interface{})
	if !ok || len(creds) == 0 {
		return fmt.Errorf("connector credentials are missing")
	}
	return nil
}

func (s *ConnectorService) DeleteConnector(connectorID, userID string) (bool, common.ErrorCode, error) {
	if connectorID == "" {
		return false, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return false, common.CodeServerError, err
	}

	if !s.canAccessConnector(connector, userID) {
		return false, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	if err = s.cancelConnectorTasks(connector.ID); err != nil {
		return false, common.CodeServerError, err
	}

	if err = s.connectorDAO.DeleteByID(connector.ID); err != nil {
		return false, common.CodeServerError, err
	}
	return true, common.CodeSuccess, nil
}

// RebuildConnector schedules a rebuild for an accessible connector and knowledge base.
func (s *ConnectorService) RebuildConnector(connectorID, userID, kbID string) (bool, common.ErrorCode, error) {
	if connectorID == "" {
		return false, common.CodeDataError, fmt.Errorf("connector_id is required")
	}
	if kbID == "" {
		return false, common.CodeArgumentError, fmt.Errorf("required argument is missing: kb_id")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return false, common.CodeServerError, err
	}

	if !s.canAccessConnector(connector, userID) {
		return false, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	sourceType := fmt.Sprintf("%s/%s", connector.Source, connector.ID)
	documents, err := s.connectorDAO.ListDocumentsByKBAndSourceType(kbID, sourceType)
	if err != nil {
		return false, common.CodeServerError, err
	}

	s.deleteConnectorDocumentChunks(connector.TenantID, kbID, documents)

	if err := s.connectorDAO.RebuildConnector(connector, kbID, documents); err != nil {
		return false, common.CodeServerError, err
	}
	return true, common.CodeSuccess, nil
}

func (s *ConnectorService) deleteConnectorDocumentChunks(tenantID, kbID string, documents []*entity.Document) {
	docEngine := engine.Get()
	if docEngine == nil {
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	for _, document := range documents {
		_, _ = docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": document.ID}, indexName, kbID)
	}
}

func (s *ConnectorService) ListLog(connectorID, userID string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, 0, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return nil, 0, common.CodeServerError, err
	}

	if !s.canAccessConnector(connector, userID) {
		return nil, 0, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 15
	}
	offset := (page - 1) * pageSize

	logs, total, err := s.connectorDAO.ListLogsByConnectorID(connectorID, offset, pageSize)
	if err != nil {
		return nil, 0, common.CodeServerError, fmt.Errorf("failed to fetch connector logs: %w", err)
	}
	if logs == nil {
		logs = []*entity.ConnectorSyncLog{}
	}
	return logs, total, common.CodeSuccess, nil
}
