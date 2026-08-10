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

package server

import (
	"ragflow/internal/common"
	"sync"
)

type LocalVariables struct {
	ServerName *string // Server name, can be modified at runtime
}

var (
	localVariables     *LocalVariables
	localVariablesOnce sync.Once
	localVariablesMu   sync.RWMutex
)

func InitLocalVariables() error {
	var initErr error
	localVariablesOnce.Do(func() {
		localVariables = &LocalVariables{}
		common.Info("Local variables initialized successfully")
	})
	return initErr
}

func SetServerName(serverName string) {
	localVariablesMu.Lock()
	defer localVariablesMu.Unlock()
	localVariables.ServerName = &serverName
}

func GetServerName() string {
	localVariablesMu.RLock()
	defer localVariablesMu.RUnlock()
	return *localVariables.ServerName
}
