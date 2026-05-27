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
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
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
	connectorDAO    *dao.ConnectorDAO
	connector2KbDAO *dao.Connector2KbDAO
	syncLogsDAO     *dao.SyncLogsDAO
	userTenantDAO   *dao.UserTenantDAO
}

// NewConnectorService create connector service
func NewConnectorService() *ConnectorService {
	return &ConnectorService{
		connectorDAO:    dao.NewConnectorDAO(),
		connector2KbDAO: dao.NewConnector2KbDAO(),
		syncLogsDAO:     dao.NewSyncLogsDAO(),
		userTenantDAO:   dao.NewUserTenantDAO(),
	}
}

// ListConnectorsResponse list connectors response
type ListConnectorsResponse struct {
	Connectors []*dao.ConnectorListItem `json:"connectors"`
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

// GetConnector returns connector details if the user can access it.
// Equivalent to Python's get_connector.
func (s *ConnectorService) GetConnector(connectorID, userID string) (*entity.Connector, error) {
	ok, err := s.accessible(connectorID, userID)
	if err != nil && errors.Is(err, ErrConnectorNotFound) {
		return nil, ErrConnectorNotFound
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrConnectorNoAuth
	}

	conn, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return nil, ErrConnectorNotFound
	}
	return conn, nil
}

// CreateConnectorRequest holds the fields accepted when creating a connector.
// Mirrors the payload consumed by Python's create_connector.
type CreateConnectorRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Source      string                 `json:"source" binding:"required"`
	Config      map[string]interface{} `json:"config"`
	RefreshFreq *int64                 `json:"refresh_freq,omitempty"`
	PruneFreq   *int64                 `json:"prune_freq,omitempty"`
	TimeoutSecs *int64                 `json:"timeout_secs,omitempty"`
}

// CreateConnector creates a connector owned by the current user's tenant.
// Equivalent to Python's create_connector (default freqs: 5/5, timeout: 60*29).
func (s *ConnectorService) CreateConnector(userID string, req *CreateConnectorRequest) (*entity.Connector, error) {
	refreshFreq := int64(5)
	if req.RefreshFreq != nil {
		refreshFreq = *req.RefreshFreq
	}
	pruneFreq := int64(5)
	if req.PruneFreq != nil {
		pruneFreq = *req.PruneFreq
	}
	timeoutSecs := int64(60 * 29)
	if req.TimeoutSecs != nil {
		timeoutSecs = *req.TimeoutSecs
	}

	config := entity.JSONMap{}
	for k, v := range req.Config {
		config[k] = v
	}

	conn := &entity.Connector{
		ID:          common.GenerateUUID(),
		TenantID:    userID,
		Name:        req.Name,
		Source:      req.Source,
		InputType:   entity.ConnectorInputTypePoll,
		Config:      config,
		RefreshFreq: refreshFreq,
		PruneFreq:   pruneFreq,
		TimeoutSecs: timeoutSecs,
		Status:      string(entity.TaskStatusUnstart),
	}

	if err := s.connectorDAO.Create(conn); err != nil {
		return nil, fmt.Errorf("failed to create connector: %w", err)
	}

	created, err := s.connectorDAO.GetByID(conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load created connector: %w", err)
	}
	return created, nil
}

// UpdateConnectorRequest holds the mutable fields and scheduling controls for an update.
// Mirrors the payload consumed by Python's update_connector.
type UpdateConnectorRequest struct {
	PruneFreq   *int64                  `json:"prune_freq,omitempty"`
	RefreshFreq *int64                  `json:"refresh_freq,omitempty"`
	TimeoutSecs *int64                  `json:"timeout_secs,omitempty"`
	Config      *map[string]interface{} `json:"config,omitempty"`
	Reschedule  *bool                   `json:"reschedule,omitempty"`
	Status      *string                 `json:"status,omitempty"`
}

// UpdateConnector updates the polling configuration of an accessible connector
// and applies any requested (re)scheduling. Equivalent to Python's update_connector.
func (s *ConnectorService) UpdateConnector(connectorID, userID string, req *UpdateConnectorRequest) (*entity.Connector, error) {
	ok, err := s.accessible(connectorID, userID)
	if err != nil && errors.Is(err, ErrConnectorNotFound) {
		return nil, ErrConnectorNotFound
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrConnectorNoAuth
	}

	if _, err := s.connectorDAO.GetByID(connectorID); err != nil {
		return nil, ErrConnectorNotFound
	}

	updates := map[string]interface{}{}
	if req.PruneFreq != nil {
		updates["prune_freq"] = *req.PruneFreq
	}
	if req.RefreshFreq != nil {
		updates["refresh_freq"] = *req.RefreshFreq
	}
	if req.TimeoutSecs != nil {
		updates["timeout_secs"] = *req.TimeoutSecs
	}
	if req.Config != nil {
		config := entity.JSONMap{}
		for k, v := range *req.Config {
			config[k] = v
		}
		updates["config"] = config
	}
	if len(updates) > 0 {
		if err := s.connectorDAO.UpdateByID(connectorID, updates); err != nil {
			return nil, fmt.Errorf("failed to update connector: %w", err)
		}
	}

	// Scheduling controls, mirroring the Python branch order:
	// reschedule wins, else CANCEL/SCHEDULE on status.
	switch {
	case req.Reschedule != nil && *req.Reschedule:
		if err := s.cancelTasks(connectorID); err != nil {
			return nil, err
		}
		if err := s.scheduleTasks(connectorID); err != nil {
			return nil, err
		}
	case req.Status != nil && isCancelStatus(*req.Status):
		if err := s.cancelTasks(connectorID); err != nil {
			return nil, err
		}
	case req.Status != nil && isScheduleStatus(*req.Status):
		if err := s.scheduleTasks(connectorID); err != nil {
			return nil, err
		}
	}

	updated, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return nil, ErrConnectorNotFound
	}
	return updated, nil
}

// DeleteConnector cancels the connector's sync tasks and deletes it.
// Equivalent to Python's rm_connector.
func (s *ConnectorService) DeleteConnector(connectorID, userID string) error {
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

	if err := s.cancelTasks(connectorID); err != nil {
		return err
	}
	return s.connectorDAO.DeleteByID(connectorID)
}

// ListLogsResponse mirrors Python's {"total": total, "logs": arr}.
type ListLogsResponse struct {
	Total int64              `json:"total"`
	Logs  []*dao.SyncLogItem `json:"logs"`
}

// ListLogs lists sync logs for an accessible connector.
// Equivalent to Python's list_logs.
func (s *ConnectorService) ListLogs(connectorID, userID string, page, pageSize int) (*ListLogsResponse, error) {
	ok, err := s.accessible(connectorID, userID)
	if err != nil && errors.Is(err, ErrConnectorNotFound) {
		return nil, ErrConnectorNotFound
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrConnectorNoAuth
	}

	logs, total, err := s.syncLogsDAO.ListByConnectorID(connectorID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &ListLogsResponse{Total: total, Logs: logs}, nil
}

// Rebuild schedules a full re-sync of a connector against a knowledge base.
// Equivalent to Python's ConnectorService.rebuild: it drops the existing sync
// logs for the pair and enqueues a fresh reindex SYNC task (plus a PRUNE task
// when sync_deleted_files is enabled).
//
// Note: the Python flow also deletes the previously indexed documents via
// FileService.delete_docs. That document-deletion path is not yet ported to Go;
// the re-sync task will re-ingest from the source, but stale documents are not
// removed here. Tracked as a follow-up alongside the task-worker port.
func (s *ConnectorService) Rebuild(connectorID, kbID, userID string) error {
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

	if err := s.syncLogsDAO.DeleteByConnectorAndKb(connectorID, kbID); err != nil {
		return err
	}

	if err := s.scheduleSync(connectorID, kbID, true, entity.ConnectorTaskTypeSync); err != nil {
		return err
	}
	if connectorPruneEnabled(conn) {
		if err := s.scheduleSync(connectorID, kbID, false, entity.ConnectorTaskTypePrune); err != nil {
			return err
		}
	}
	return nil
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

// cancelTasks marks scheduled/running sync logs of the connector as cancelled and
// sets the connector status to CANCEL. Mirrors Python's ConnectorService.cancel_tasks.
func (s *ConnectorService) cancelTasks(connectorID string) error {
	if _, err := s.connectorDAO.GetByID(connectorID); err != nil {
		return nil
	}

	mappings, err := s.connector2KbDAO.ListByConnectorID(connectorID)
	if err != nil {
		return err
	}
	active := []string{string(entity.TaskStatusSchedule), string(entity.TaskStatusRunning)}
	for _, m := range mappings {
		if err := s.syncLogsDAO.UpdateStatusByConnectorAndKb(connectorID, m.KbID, active, string(entity.TaskStatusCancel)); err != nil {
			return err
		}
	}
	return s.connectorDAO.UpdateByID(connectorID, map[string]interface{}{"status": string(entity.TaskStatusCancel)})
}

// scheduleTasks enqueues SYNC (and PRUNE when enabled) tasks for each linked
// knowledge base. Mirrors Python's ConnectorService.schedule_tasks.
func (s *ConnectorService) scheduleTasks(connectorID string) error {
	conn, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return nil
	}

	mappings, err := s.connector2KbDAO.ListByConnectorID(connectorID)
	if err != nil {
		return err
	}
	pruneEnabled := connectorPruneEnabled(conn)
	for _, m := range mappings {
		if err := s.scheduleSync(connectorID, m.KbID, false, entity.ConnectorTaskTypeSync); err != nil {
			return err
		}
		if pruneEnabled {
			if err := s.scheduleSync(connectorID, m.KbID, false, entity.ConnectorTaskTypePrune); err != nil {
				return err
			}
		}
	}
	return nil
}

// scheduleSync inserts a SCHEDULE-status sync log for the connector/kb pair if one
// of the same task type is not already scheduled, and flips the connector status to
// SCHEDULE. Mirrors the core of Python's SyncLogsService.schedule.
func (s *ConnectorService) scheduleSync(connectorID, kbID string, reindex bool, taskType string) error {
	scheduled, err := s.syncLogsDAO.HasScheduled(connectorID, kbID, taskType, string(entity.TaskStatusSchedule))
	if err != nil {
		return err
	}
	if scheduled {
		return nil
	}

	fromBeginning := "0"
	if reindex {
		fromBeginning = "1"
	}
	log := &entity.SyncLogs{
		ID:            common.GenerateUUID(),
		ConnectorID:   connectorID,
		KbID:          kbID,
		TaskType:      taskType,
		Status:        string(entity.TaskStatusSchedule),
		FromBeginning: &fromBeginning,
	}
	if err := s.syncLogsDAO.Create(log); err != nil {
		return err
	}
	return s.connectorDAO.UpdateByID(connectorID, map[string]interface{}{"status": string(entity.TaskStatusSchedule)})
}

// connectorPruneEnabled reports whether the connector opted into deleted-file pruning.
func connectorPruneEnabled(conn *entity.Connector) bool {
	if conn == nil || conn.Config == nil {
		return false
	}
	v, ok := conn.Config["sync_deleted_files"]
	if !ok {
		return false
	}
	enabled, _ := v.(bool)
	return enabled
}

// isCancelStatus matches Python's check against TaskStatus.CANCEL / "CANCEL".
func isCancelStatus(status string) bool {
	return status == string(entity.TaskStatusCancel) || status == "CANCEL"
}

// isScheduleStatus matches Python's check against TaskStatus.SCHEDULE / "SCHEDULE".
func isScheduleStatus(status string) bool {
	return status == string(entity.TaskStatusSchedule) || status == "SCHEDULE"
}
