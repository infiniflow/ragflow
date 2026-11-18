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

import (
	"fmt"

	"github.com/spf13/viper"

	"ragflow/internal/common"
)

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
	Level      string `mapstructure:"level"`       // debug, info, warn, error; default "warn"
	Format     string `mapstructure:"format"`      // json, text (reserved for future use); default "json"
	Path       string `mapstructure:"path"`        // per-binary file override; empty = use cmd/* hardcoded default
	MaxSize    int    `mapstructure:"max_size"`    // MB before rotation; default 100
	MaxBackups int    `mapstructure:"max_backups"` // retained rotated files; default 10
	MaxAge     int    `mapstructure:"max_age"`     // days; default 30
	Compress   *bool  `mapstructure:"compress"`    // gzip rotated files; nil = project default (true)
}

func (c *Config) ParseLogConfig(v *viper.Viper) error {
	// Default Log config. Rotation defaults come from common so they stay
	// in sync with the logger's own fallback values.
	c.log.Level = "warn"
	c.log.Format = "json"
	c.log.Path = "logs"
	c.log.MaxSize = common.DefaultLogMaxSizeMB
	c.log.MaxBackups = common.DefaultLogMaxBackups
	c.log.MaxAge = common.DefaultLogMaxAgeDays
	c.log.Compress = boolPtr(common.DefaultLogCompress)

	// Read each log.* key from the ROOT viper (which carries the
	// prefix/replacer/AutomaticEnv configured in server.Init) rather than
	// from v.Sub("log"). viper's Sub returns nil for an env-only key, so a
	// lone RAGFLOW_LOG_LEVEL (no `log:` YAML section) would otherwise be
	// silently dropped. Reading from the root honours the env override in
	// both the config-file and env-only cases, and env wins over the file
	// (AutomaticEnv precedence).
	if v.IsSet("log.level") {
		c.log.Level = v.GetString("log.level")
	}

	if v.IsSet("log.format") {
		c.log.Format = v.GetString("log.format")
	}

	if v.IsSet("log.path") {
		c.log.Path = v.GetString("log.path")
	}

	if v.IsSet("log.max_size") {
		c.log.MaxSize = v.GetInt("log.max_size")
	}

	if v.IsSet("log.max_backups") {
		c.log.MaxBackups = v.GetInt("log.max_backups")
	}

	if v.IsSet("log.max_age") {
		c.log.MaxAge = v.GetInt("log.max_age")
	}

	if v.IsSet("log.compress") {
		compress := v.GetBool("log.compress")
		c.log.Compress = &compress
	}

	return nil
}

// boolPtr returns a pointer to the given bool, used to populate *bool config
// fields so that "not set" (nil) is distinguishable from an explicit value.
func boolPtr(b bool) *bool {
	return &b
}

func (c *Config) GetLogConfig() LogConfig {
	return c.log
}

func (c *Config) SetLogLevel(level string) {
	c.log.Level = level
}

// ResolveLogLevel resolves the effective log level from the precedence
// CLI flag > config file (viper, already merged with the environment) >
// default ("warn"), and validates the final value against the supported
// set. An invalid level from any layer is a fatal startup error rather
// than a silent downgrade: common.InitLogger falls back to info for
// unknown levels, which would hide the misconfiguration. The CLI parser
// still rejects an invalid --log-level up front, so a bad CLI value fails
// with a message naming the flag; this check owns the config/env layers.
func (c *Config) ResolveLogLevel(cli *string) (string, error) {
	level := c.log.Level
	if cli != nil {
		level = *cli
	}
	if level == "" {
		level = "warn"
	}
	switch level {
	case "debug", "info", "warn", "error":
		return level, nil
	default:
		return "", fmt.Errorf("invalid log level %q: must be debug, info, warn, or error (check --log-level or the log.level config key)", level)
	}
}
