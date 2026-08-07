//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package syncer

import "sync"

// ConnectorLocker serializes work for a connector.
type ConnectorLocker interface {
	TryLock(connectorID string) bool
	Unlock(connectorID string)
}

// ConnectorLock is a process-local connector mutex registry.
type ConnectorLock struct {
	mu     sync.Mutex
	locked map[string]struct{}
}

// NewConnectorLock creates an empty process-local connector lock.
func NewConnectorLock() *ConnectorLock {
	return &ConnectorLock{locked: map[string]struct{}{}}
}

// TryLock attempts to acquire the connector lock without blocking.
func (l *ConnectorLock) TryLock(connectorID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.locked[connectorID]; ok {
		return false
	}
	l.locked[connectorID] = struct{}{}
	return true
}

// Unlock releases the connector lock.
func (l *ConnectorLock) Unlock(connectorID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.locked, connectorID)
}
