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
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"strings"
	"time"

	"ragflow/internal/common"
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

// UpdateConnectorRequest update connector request.
type UpdateConnectorRequest struct {
	RefreshFreq *int64                  `json:"refresh_freq,omitempty"`
	PruneFreq   *int64                  `json:"prune_freq,omitempty"`
	Config      *map[string]interface{} `json:"config,omitempty"`
	TimeoutSecs *int64                  `json:"timeout_secs,omitempty"`
	Reschedule  bool                    `json:"reschedule,omitempty"`
	Status      string                  `json:"status,omitempty"`
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

// UpdateConnector updates an accessible connector's polling configuration.
// Equivalent to Python's update_connector in api/apps/restful_apis/connector_api.py.
func (s *ConnectorService) UpdateConnector(connectorID, userID string, req *UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	if strings.TrimSpace(connectorID) == "" {
		return nil, common.CodeDataError, errors.New("connector_id is required")
	}
	if !s.accessible(connectorID, userID) {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Can't find this Connector!")
	}

	if req != nil {
		updates := map[string]interface{}{}
		if req.PruneFreq != nil {
			updates["prune_freq"] = *req.PruneFreq
		}
		if req.RefreshFreq != nil {
			updates["refresh_freq"] = *req.RefreshFreq
		}
		if req.Config != nil {
			updates["config"] = entity.JSONMap(*req.Config)
		}
		if req.TimeoutSecs != nil {
			updates["timeout_secs"] = *req.TimeoutSecs
		}
		if len(updates) > 0 {
			if err := s.connectorDAO.UpdateByID(connectorID, updates); err != nil {
				return nil, common.CodeServerError, err
			}
		}

		switch {
		case req.Reschedule:
			if err := s.cancelTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
			if err := s.scheduleTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
		case req.Status == string(entity.TaskStatusCancel) || strings.EqualFold(req.Status, "CANCEL"):
			if err := s.cancelTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
		case req.Status == string(entity.TaskStatusSchedule) || strings.EqualFold(req.Status, "SCHEDULE"):
			if err := s.scheduleTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
		}
	}

	connector, err = s.connectorDAO.GetByID(connector.ID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Can't find this Connector!")
	}
	return connector, common.CodeSuccess, nil
}

func (s *ConnectorService) accessible(connectorID, userID string) bool {
	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return false
	}
	if connector.TenantID == userID {
		return true
	}
	_, err = s.userTenantDAO.FilterByUserIDAndTenantID(userID, connector.TenantID)
	return err == nil
}

func (s *ConnectorService) cancelTasks(connectorID string) error {
	if err := s.connectorDAO.CancelRunningOrScheduledLogs(connectorID); err != nil {
		return err
	}
	return s.connectorDAO.UpdateByID(connectorID, map[string]interface{}{"status": string(entity.TaskStatusCancel)})
}

func (s *ConnectorService) scheduleTasks(connectorID string) error {
	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return err
	}

	mappings, err := s.connectorDAO.ListMappingsByConnectorID(connectorID)
	if err != nil {
		return err
	}

	pruneEnabled, _ := connector.Config["sync_deleted_files"].(bool)
	for _, mapping := range mappings {
		pollRangeStart, totalDocsIndexed, err := s.latestDoneSyncState(connectorID, mapping.KbID)
		if err != nil {
			return err
		}
		if err := s.scheduleTask(connectorID, mapping.KbID, "sync", pollRangeStart, totalDocsIndexed); err != nil {
			return err
		}
		if pruneEnabled {
			if err := s.scheduleTask(connectorID, mapping.KbID, "prune", nil, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ConnectorService) latestDoneSyncState(connectorID, datasetID string) (*string, int64, error) {
	latest, err := s.connectorDAO.GetLatestDoneSyncLog(connectorID, datasetID, "sync")
	if err != nil {
		if dao.IsRecordNotFound(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	return latest.PollRangeEnd, latest.TotalDocsIndexed, nil
}

func (s *ConnectorService) scheduleTask(connectorID, datasetID, taskType string, pollRangeStart *string, totalDocsIndexed int64) error {
	fromBeginning := "0"
	now := time.Now().Local()
	return s.connectorDAO.ScheduleSyncLogIfAbsent(&entity.SyncLogs{
		ID:               common.GenerateUUID(),
		ConnectorID:      connectorID,
		KbID:             datasetID,
		TaskType:         taskType,
		Status:           string(entity.TaskStatusSchedule),
		PollRangeStart:   pollRangeStart,
		FromBeginning:    &fromBeginning,
		TotalDocsIndexed: totalDocsIndexed,
		TimeStarted:      &now,
	})
}
