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

package admin

import (
	"errors"
	"ragflow/internal/common"
	"sync"
	"time"
)

// Service errors
var (
	ErrInvalidToken = errors.New("invalid token")
	ErrNotAdmin     = errors.New("user is not admin")
	ErrUserInactive = errors.New("user is inactive")
	ErrUserNotFound = errors.New("user not found")
)

// API server state

// ServerStore is a thread-safe global server status storage
type ServerStore struct {
	mu      sync.RWMutex
	servers map[string]*common.BaseMessage // key: server_id
}

// GlobalServerStore is the global instance
var GlobalServerStore = &ServerStore{
	servers: make(map[string]*common.BaseMessage),
}

// UpdateServerInfo updates or adds a server status
func (s *ServerStore) UpdateServerInfo(serverName string, status *common.BaseMessage) {

	//switch serviceType {
	//case "meta_data":
	//	return s.getMySQLStatus(name)

	switch status.ServerType {
	case common.ServerTypeAPI:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.servers[serverName] = status
		return
	case common.ServerTypeIngestion:
		return
	}
}

// GetServerInfo gets a single server status
func (s *ServerStore) GetServerInfo(serverName string) (*common.BaseMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.servers[serverName]
	return status, ok
}

// ListInfos gets all server infos
func (s *ServerStore) ListInfos() []*common.BaseMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*common.BaseMessage, 0, len(s.servers))
	for _, status := range s.servers {
		result = append(result, status)
	}
	return result
}

// ListInfosByType gets server infos by type
func (s *ServerStore) ListInfosByType(serverType common.ServerType) []*common.BaseMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*common.BaseMessage, 0)
	for _, status := range s.servers {
		if status.ServerType == serverType {
			result = append(result, status)
		}
	}
	return result
}

// RemoveStatus removes a server status
func (s *ServerStore) RemoveStatus(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.servers, serverID)
}

// CleanupStaleStatuses cleans up servers that haven't reported for a specified duration
func (s *ServerStore) CleanupStaleStatuses(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, status := range s.servers {
		if now.Sub(status.Timestamp) > maxAge {
			delete(s.servers, id)
		}
	}
}
