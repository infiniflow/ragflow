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
	PollInterval           time.Duration
	TaskConcurrency        int
	TaskQueueSize          int
	PerTaskItemConcurrency int
	GlobalItemConcurrency  int
	ItemRetryCount         int
	ItemRetryBaseDelay     time.Duration
}

// DefaultConfig returns the first-version syncer defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval:           3 * time.Second,
		TaskConcurrency:        3,
		TaskQueueSize:          32,
		PerTaskItemConcurrency: 4,
		GlobalItemConcurrency:  12,
		ItemRetryCount:         3,
		ItemRetryBaseDelay:     time.Second,
	}
}

// Normalize fills unset or invalid config values with defaults.
func (c Config) Normalize() Config {
	def := DefaultConfig()
	if c.PollInterval <= 0 {
		c.PollInterval = def.PollInterval
	}

	if c.TaskConcurrency <= 0 {
		c.TaskConcurrency = def.TaskConcurrency
	}

	if c.TaskQueueSize <= 0 {
		c.TaskQueueSize = def.TaskQueueSize
	}

	if c.PerTaskItemConcurrency <= 0 {
		c.PerTaskItemConcurrency = def.PerTaskItemConcurrency
	}

	if c.GlobalItemConcurrency <= 0 {
		c.GlobalItemConcurrency = def.GlobalItemConcurrency
	}

	if c.ItemRetryCount <= 0 {
		c.ItemRetryCount = def.ItemRetryCount
	}

	if c.ItemRetryBaseDelay <= 0 {
		c.ItemRetryBaseDelay = def.ItemRetryBaseDelay
	}

	return c
}
