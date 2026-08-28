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
	"time"
)

// Config contains runtime limits for the datasource syncer.
type Config struct {
	TaskWorkerCount       int
	TaskQueueSize         int
	JobWorkerCount        int
	JobQueueSize          int
	ItemRetryCount        int
	ItemRetryBaseDelay    time.Duration
	MaxAnchorRestartCount int
}

// DefaultConfig returns the first-version syncer defaults.
func DefaultConfig() Config {
	return Config{
		TaskWorkerCount:       5,
		TaskQueueSize:         10,
		JobWorkerCount:        400,
		JobQueueSize:          450,
		ItemRetryCount:        3,
		ItemRetryBaseDelay:    time.Second,
		MaxAnchorRestartCount: 2,
	}
}

// Normalize fills unset or invalid config values with defaults.
func (c Config) Normalize() Config {
	def := DefaultConfig()
	if c.TaskWorkerCount <= 0 {
		c.TaskWorkerCount = def.TaskWorkerCount
	}

	if c.TaskQueueSize <= 0 {
		c.TaskQueueSize = def.TaskQueueSize
	}

	if c.JobWorkerCount <= 0 {
		c.JobWorkerCount = def.JobWorkerCount
	}

	if c.JobQueueSize <= 0 {
		c.JobQueueSize = def.JobQueueSize
	}

	if c.ItemRetryCount <= 0 {
		c.ItemRetryCount = def.ItemRetryCount
	}

	if c.ItemRetryBaseDelay <= 0 {
		c.ItemRetryBaseDelay = def.ItemRetryBaseDelay
	}

	if c.MaxAnchorRestartCount <= 0 {
		c.MaxAnchorRestartCount = def.MaxAnchorRestartCount
	}

	return c
}
