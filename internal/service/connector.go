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

	"gorm.io/gorm"
)

const (
	connectorInputTypePoll   = "poll"
	connectorStatusUnstarted = "0"
	defaultConnectorFreq     = 5
	defaultConnectorTimeout  = 60 * 29
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
func (s *ConnectorService) GetConnector(connectorID string, userID string) (*entity.Connector, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
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

// GetConnector returns connector details when the current user can access it.
// Equivalent to Python's get_connector in api/apps/restful_apis/connector_api.py.
func (s *ConnectorService) GetConnector(connectorID, userID string) (*entity.Connector, common.ErrorCode, error) {
	if strings.TrimSpace(connectorID) == "" {
		return nil, common.CodeDataError, errors.New("connector_id is required")
	}
	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Can't find this Connector!")
	}
	if !s.canAccessConnector(connector, userID) {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}
	return connector, common.CodeSuccess, nil
}

// UpdateConnector updates connector settings for an accessible connector.
func (s *ConnectorService) UpdateConnector(connectorID, userID string, req *UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	if strings.TrimSpace(connectorID) == "" {
		return nil, common.CodeDataError, errors.New("connector_id is required")
	}
	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Can't find this Connector!")
	}
	if !s.canAccessConnector(connector, userID) {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
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
		if req.Reschedule {
			updates["status"] = string(entity.TaskStatusSchedule)
		}
		if req.Status != "" {
			switch {
			case strings.EqualFold(req.Status, "cancel"):
				updates["status"] = string(entity.TaskStatusCancel)
			case strings.EqualFold(req.Status, "schedule"):
				updates["status"] = string(entity.TaskStatusSchedule)
			default:
				updates["status"] = req.Status
			}
		}
		if len(updates) > 0 {
			if err := s.connectorDAO.UpdateByID(connectorID, updates); err != nil {
				return nil, common.CodeServerError, errors.New("failed to update connector")
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
	return s.canAccessConnector(connector, userID)
}

func (s *ConnectorService) canAccessConnector(connector *entity.Connector, userID string) bool {
	if connector.TenantID == userID {
		return true
	}
	_, err := s.userTenantDAO.FilterByUserIDAndTenantID(userID, connector.TenantID)
	return err == nil
}
