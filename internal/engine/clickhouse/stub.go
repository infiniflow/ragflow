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

//go:build !ee

package clickhouse

import "ragflow/internal/common"

// Driver is the analytics sink. The OSS build never opens a ClickHouse
// connection, so every method drops what it is given.
type Driver struct{}

// GetDriver returns the analytics sink, which the OSS build never creates.
func GetDriver() *Driver { return nil }

// CollectModelUsage records token usage for analytics.
func (d *Driver) CollectModelUsage(modelUsage *common.ModelUsage) error { return nil }

// SaveOperationLog records an operation log entry.
func (d *Driver) SaveOperationLog(operationLog *common.OperationLog) error { return nil }

// Status reports the analytics backend health.
func (d *Driver) Status() (map[string]interface{}, error) { return nil, nil }
