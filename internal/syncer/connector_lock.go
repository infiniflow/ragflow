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
	"context"
	"fmt"
	"ragflow/internal/engine/redis"
	"ragflow/internal/utility"
	"sync"
	"time"
)

// ConnectorLocker serializes work for one connector and knowledge base.
type ConnectorLocker interface {
	TryLock(connectorID, kbID string) (ConnectorLockLease, bool)
	Unlock(connectorID, kbID string)
}

const connectorLockTTL = 24 * time.Hour

// ConnectorLockLease describes the bounded lifetime of a connector/KB lock.
type ConnectorLockLease struct {
	ExpiresAt time.Time
}

// ConnectorLock serializes connector/KB work through Redis when available.
type ConnectorLock struct {
	holder string
	mu     sync.Mutex
	local  map[string]struct{}
	redis  map[string]*redis.DistributedLock
}

// NewConnectorLock creates an empty connector/KB lock.
func NewConnectorLock() *ConnectorLock {
	return &ConnectorLock{holder: utility.GenerateUUID(), local: map[string]struct{}{}, redis: map[string]*redis.DistributedLock{}}
}

// TryLock attempts to acquire the connector/KB lock without blocking.
func (l *ConnectorLock) TryLock(connectorID, kbID string) (ConnectorLockLease, bool) {
	if l == nil {
		return ConnectorLockLease{}, false
	}
	key := connectorLockKey(connectorID, kbID)
	l.mu.Lock()
	if _, ok := l.local[key]; ok {
		l.mu.Unlock()
		return ConnectorLockLease{}, false
	}
	l.local[key] = struct{}{}
	l.mu.Unlock()

	if client := redis.Get(); client != nil {
		lock := redis.NewDistributedLock(key, l.holder, connectorLockTTL, 0)
		if lock == nil || !lock.Acquire(context.Background()) {
			l.mu.Lock()
			delete(l.local, key)
			l.mu.Unlock()
			return ConnectorLockLease{}, false
		}
		l.mu.Lock()
		l.redis[key] = lock
		l.mu.Unlock()
		return newConnectorLockLease(), true
	}
	return newConnectorLockLease(), true
}

// Unlock releases the connector/KB lock.
func (l *ConnectorLock) Unlock(connectorID, kbID string) {
	if l == nil {
		return
	}
	key := connectorLockKey(connectorID, kbID)
	l.mu.Lock()
	lock := l.redis[key]
	delete(l.redis, key)
	delete(l.local, key)
	l.mu.Unlock()
	if lock != nil {
		lock.Release(context.Background())
	}
}

func connectorLockKey(connectorID, kbID string) string {
	return fmt.Sprintf("syncer:connector-lock:%s:%s", connectorID, kbID)
}

func newConnectorLockLease() ConnectorLockLease {
	return ConnectorLockLease{ExpiresAt: time.Now().Add(connectorLockTTL)}
}
