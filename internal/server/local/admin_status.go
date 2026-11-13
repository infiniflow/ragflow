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

package local

import (
	"fmt"
	"ragflow/internal/common"
	"sync"
)

// AdminStatus represents the admin status
// 0 = valid, 1 = invalid

type AdminStatusCode int

const (
	Available      AdminStatusCode = 0
	Unavailable    AdminStatusCode = 1
	NotInitialized AdminStatusCode = 10
)

type AdminStatus struct {
	Status AdminStatusCode `json:"status"` // 0 = available, 1 = not available
	Reason string          `json:"reason"` // reason for invalid status
}

var (
	adminStatus     *AdminStatus
	adminStatusMu   sync.RWMutex
	adminStatusOnce sync.Once
)

// InitAdminStatus initializes the global admin status
// status: 0 = valid, 1 = invalid (default)
func InitAdminStatus() {
	adminStatusOnce.Do(func() {
		adminStatus = &AdminStatus{
			Status: NotInitialized,
			Reason: "admin server not connected",
		}
	})
}

// GetAdminStatus returns the current admin status
func GetAdminStatus() AdminStatus {
	adminStatusMu.RLock()
	defer adminStatusMu.RUnlock()
	if adminStatus == nil {
		return AdminStatus{Status: 1, Reason: "not initialized"}
	}
	return AdminStatus{
		Status: adminStatus.Status,
		Reason: adminStatus.Reason,
	}
}

// SetAdminStatus updates the admin status
func SetAdminStatus(statusCode AdminStatusCode, reason string) {
	adminStatusMu.Lock()
	defer adminStatusMu.Unlock()
	if adminStatus == nil {
		common.Fatal("Admin server status not initialized")
	}

	if adminStatus.Status == statusCode {
		return
	}

	switch statusCode {
	case Available:
		common.Warn(fmt.Sprintf("Admin server is available"))
	case Unavailable:
		common.Warn(fmt.Sprintf("Admin server is unavailable, code: %d, reason: %s", statusCode, reason))
	default:
		common.Fatal(fmt.Sprintf("Invalid admin status code: %d, reason: %s", statusCode, reason))
		return
	}

	adminStatus.Status = statusCode
	adminStatus.Reason = reason
}
