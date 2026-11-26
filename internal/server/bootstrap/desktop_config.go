//go:build desktop

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

package bootstrap

import (
	"net"
	"path/filepath"
	"strconv"

	"ragflow/internal/server/config"
)

// DesktopEndpoints are the loopback addresses the desktop build's in-process
// backends listen on, plus the directory that owns their files.
type DesktopEndpoints struct {
	KvrocksAddr     string
	KvrocksPassword string
	NATSHost        string
	NATSPort        int
	S3Endpoint      string
	S3AccessKey     string
	S3SecretKey     string
	InfinityAddr    string
	DataDir         string
}

var desktopEndpoints *DesktopEndpoints

// SetDesktopEndpoints records the endpoints internal/desktop started. It has to
// run before the server parses its configuration, so ApplyConfigOverrides can
// rewrite the parsed values instead of relying on environment variables: viper's
// nested lookups (kvrocks.host, nats.host) do not see env overrides.
func SetDesktopEndpoints(ep DesktopEndpoints) { desktopEndpoints = &ep }

// ApplyConfigOverrides points the cache, queue and storage settings at the
// in-process backends. Everything else (database, document engine) is left as
// configured: the desktop build does not start those itself.
func ApplyConfigOverrides(cfg *config.Config) {
	if desktopEndpoints == nil {
		return
	}

	kvrocks := cfg.GetKvrocksConfig()
	if host, port, err := net.SplitHostPort(desktopEndpoints.KvrocksAddr); err == nil {
		kvrocks.Host = host
		if p, err := strconv.Atoi(port); err == nil {
			kvrocks.Port = p
		}
	}
	kvrocks.Password = desktopEndpoints.KvrocksPassword
	cfg.SetKvrocksConfig(kvrocks)

	cfg.SetNATSConfig(config.NATSConfig{
		Host: desktopEndpoints.NATSHost,
		Port: desktopEndpoints.NATSPort,
	})

	cfg.SetMinioEndpoint(desktopEndpoints.S3Endpoint, desktopEndpoints.S3AccessKey, desktopEndpoints.S3SecretKey)

	// A desktop installation has exactly one metadata store and exactly one
	// document engine, both in this process. Pinning the type here (rather than
	// shipping a config file an operator could edit) is what makes them the
	// only options: the environment overrides are cleared as well, so a stray
	// DB_TYPE or DOC_ENGINE cannot point the process at a server.
	cfg.ForceDatabaseType("sqlite")
	cfg.SetSQLitePath(filepath.Join(desktopEndpoints.DataDir, "metadata.db"))
	cfg.ForceDocEngineType("infinity")

	infinity := cfg.GetInfinityConfig()
	infinity.URI = desktopEndpoints.InfinityAddr
	cfg.SetInfinityConfig(infinity)
}
