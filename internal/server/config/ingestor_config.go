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

package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type IngestorConfig struct {
	// MaxConcurrentWorkers bounds how many ingestion tasks the ingestor runs in
	// parallel (the task channel width and dataset-level compile worker count
	// default to this value). 0/negative falls back to runtime.NumCPU().
	MaxConcurrentWorkers int `mapstructure:"max_concurrent_workers"`
	// CompilerPoolSize bounds the process-wide knowledge-compilation worker
	// pool that drives the cross-doc KNN / LLM-merge / write stages. 0/negative
	// falls back to runtime.NumCPU() (or KC_COMPILE_CONCURRENCY if set).
	CompilerPoolSize int `mapstructure:"compiler_pool_size"`
	// ClaimTTL is the maximum interval a worker may go without renewing its
	// ingestion-task lease. It must leave at least three heartbeat intervals
	// of margin so normal scheduler jitter cannot look like a dead worker.
	ClaimTTL time.Duration `mapstructure:"claim_ttl"`
	// MaxLeaseRecoveryAttempts bounds automatic recovery of an expired worker
	// lease before the task is marked failed as a poison task.
	MaxLeaseRecoveryAttempts int `mapstructure:"max_lease_recovery_attempts"`
}

func (c *Config) ParseIngestorConfig(v *viper.Viper) error {
	// Default Ingestor config
	c.ingestor.MaxConcurrentWorkers = 1
	c.ingestor.CompilerPoolSize = 0
	c.ingestor.ClaimTTL = 15 * time.Second
	c.ingestor.MaxLeaseRecoveryAttempts = 3

	if !v.IsSet("ingestor") {
		return nil
	}
	sub := v.Sub("ingestor")
	if sub == nil {
		return nil
	}

	if sub.IsSet("max_concurrent_workers") {
		c.ingestor.MaxConcurrentWorkers = sub.GetInt("max_concurrent_workers")
	}

	if sub.IsSet("compiler_pool_size") {
		c.ingestor.CompilerPoolSize = sub.GetInt("compiler_pool_size")
	}

	if sub.IsSet("claim_ttl") {
		c.ingestor.ClaimTTL = sub.GetDuration("claim_ttl")
	}
	if sub.IsSet("max_lease_recovery_attempts") {
		c.ingestor.MaxLeaseRecoveryAttempts = sub.GetInt("max_lease_recovery_attempts")
	}

	heartbeat := c.general.HeartbeatInterval
	if heartbeat <= 0 {
		heartbeat = 3 * time.Second
	}
	if c.ingestor.ClaimTTL <= 3*heartbeat {
		return fmt.Errorf("ingestor claim_ttl %s must be greater than three heartbeat intervals (%s)", c.ingestor.ClaimTTL, 3*heartbeat)
	}
	if c.ingestor.MaxLeaseRecoveryAttempts <= 0 {
		return fmt.Errorf("ingestor max_lease_recovery_attempts must be positive")
	}

	return nil
}

func (c *Config) GetIngestorConfig() *IngestorConfig {
	return &c.ingestor
}
