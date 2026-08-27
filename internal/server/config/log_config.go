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

// LogConfig logging configuration.
//
// Path, MaxSize, MaxBackups, MaxAge, and Compress configure the rotated
// log file. The cmd/* entry points hardcode per-service defaults
// (e.g. "server_main.log" for the API server, "admin_server.log" for
// the admin server, "ingestion_server.log" for the ingestion worker),
// so a typical deployment gets a rotated file without any YAML
// configuration. When Path is empty (the default) the binary's
// hardcoded default filename is used — it does NOT disable file
// output. Set log.path in service_conf.yaml to override the
// per-service default filename.
//
// Compress is a pointer so callers can distinguish "not set" (nil,
// defaults to true) from "explicitly false" (*bool=false). All other
// numeric fields use plain int because their zero values are sensible
// defaults (100 MB / 10 files / 30 days) and there is no operator-meaningful
// reason to distinguish "not set" from "0".
type LogConfig struct {
	Level      string `mapstructure:"level"`       // debug, info, warn, error; default "info"
	Format     string `mapstructure:"format"`      // json, text (reserved for future use); default "json"
	Path       string `mapstructure:"path"`        // per-binary file override; empty = use cmd/* hardcoded default
	MaxSize    int    `mapstructure:"max_size"`    // MB before rotation; default 100
	MaxBackups int    `mapstructure:"max_backups"` // retained rotated files; default 10
	MaxAge     int    `mapstructure:"max_age"`     // days; default 30
	Compress   bool   `mapstructure:"compress"`    // gzip rotated files; default false
}

func (c *Config) ParseLogConfig(v *viper.Viper) error {
	// Default Log config
	c.log.Level = "info"
	c.log.Format = "json"
	c.log.Path = "logs"
	c.log.MaxSize = 100 * 1024 * 1024 // 1024MB
	c.log.MaxBackups = 10
	c.log.MaxAge = 30
	c.log.Compress = false

	if !v.IsSet("log") {
		return nil
	}
	sub := v.Sub("log")
	if sub == nil {
		return nil
	}

	if sub.IsSet("level") {
		c.log.Level = sub.GetString("level")
	}

	if sub.IsSet("format") {
		c.log.Format = sub.GetString("format")
	}

	if sub.IsSet("path") {
		c.log.Path = sub.GetString("path")
	}

	if sub.IsSet("max_size") {
		c.log.MaxSize = sub.GetInt("max_size")
	}

	if sub.IsSet("max_backups") {
		c.log.MaxBackups = sub.GetInt("max_backups")
	}

	if sub.IsSet("max_age") {
		c.log.MaxAge = sub.GetInt("max_age")
	}

	if sub.IsSet("compress") {
		c.log.Compress = sub.GetBool("compress")
	}

	return nil
}

func (c *Config) GetLogConfig() LogConfig {
	return c.log
}

func (c *Config) SetLogLevel(level string) {
	c.log.Level = level
}
