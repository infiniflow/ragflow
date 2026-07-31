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

package config

import "github.com/spf13/viper"

type SyncerConfig struct {
	MaxConcurrentSyncs int `mapstructure:"max_concurrent_syncs"`
	SyncInterval       int `mapstructure:"sync_interval"`
}

func (c *Config) ParseSyncerConfig(v *viper.Viper) error {
	// Default Syncer config
	c.syncer.MaxConcurrentSyncs = 1
	c.syncer.SyncInterval = 3

	if !v.IsSet("file_syncer") {
		return nil
	}
	sub := v.Sub("file_syncer")
	if sub == nil {
		return nil
	}

	if sub.IsSet("max_concurrent_syncs") {
		c.syncer.MaxConcurrentSyncs = sub.GetInt("max_concurrent_syncs")
	}

	if sub.IsSet("sync_interval") {
		c.syncer.SyncInterval = sub.GetInt("sync_interval")
	}

	return nil
}

func (c *Config) GetSyncerConfig() *SyncerConfig {
	return &c.syncer
}
