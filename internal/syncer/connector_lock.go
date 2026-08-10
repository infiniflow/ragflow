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

import (
	"fmt"
	"sync"
)

// ConnectorLocker serializes work for one connector and knowledge base.
type ConnectorLocker interface {
	TryLock(connectorID, kbID string) bool
	Unlock(connectorID, kbID string)
}

// ConnectorLock is a process-local connector/KB mutex registry.
type ConnectorLock struct {
	mu     sync.Mutex
	locked map[string]struct{}
}

// NewConnectorLock creates an empty process-local connector/KB lock.
func NewConnectorLock() *ConnectorLock {
	return &ConnectorLock{locked: map[string]struct{}{}}
}

// TryLock attempts to acquire the connector/KB lock without blocking.
func (l *ConnectorLock) TryLock(connectorID, kbID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := connectorLockKey(connectorID, kbID)
	if _, ok := l.locked[key]; ok {
		return false
	}
	l.locked[key] = struct{}{}
	return true
}

// Unlock releases the connector/KB lock.
func (l *ConnectorLock) Unlock(connectorID, kbID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.locked, connectorLockKey(connectorID, kbID))
}

func connectorLockKey(connectorID, kbID string) string {
	return fmt.Sprintf("%s/%s", connectorID, kbID)
}
