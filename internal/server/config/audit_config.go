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

type AuditConfig struct {
	Type       string `mapstructure:"type"`        // file, database; default "file"
	Format     string `mapstructure:"format"`      // json, text (reserved for future use); default "json"
	Path       string `mapstructure:"path"`        // per-binary file override; empty = use cmd/* hardcoded default
	MaxSize    int    `mapstructure:"max_size"`    // MB before rotation; default 100
	MaxBackups int    `mapstructure:"max_backups"` // retained rotated files; default 10
	MaxAge     int    `mapstructure:"max_age"`     // days; default 30
}
