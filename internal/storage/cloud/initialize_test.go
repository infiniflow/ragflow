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

package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"ragflow/internal/server/config"
	"ragflow/internal/storage"
)

func TestInitInstallsIntoSharedStorageFactory(t *testing.T) {
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	defer factory.SetStorage(previous)
	sentinel := storage.NewMemoryStorage()
	factory.SetStorage(sentinel)

	values := viper.New()
	values.Set("general.storage_engine", "minio")
	values.Set("minio.host", "127.0.0.1:1")
	values.Set("minio.user", "synthetic-user")
	values.Set("minio.password", "synthetic-password")
	cfg := &config.Config{}
	if err := cfg.ParseGeneralConfig(values); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ParseStorageEngineConfig(values); err != nil {
		t.Fatal(err)
	}
	if err := Init(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	installed := factory.GetStorage()
	if _, ok := installed.(*MinioStorage); !ok {
		t.Fatalf("shared factory contains %T, want *MinioStorage", installed)
	}
	if installed == sentinel {
		t.Fatal("successful initialization did not replace the backend")
	}
	if err := storage.CloseStorage(); err != nil {
		t.Fatal(err)
	}
}

func TestInitFailurePreservesInstalledBackend(t *testing.T) {
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	defer factory.SetStorage(previous)
	sentinel := storage.NewMemoryStorage()
	factory.SetStorage(sentinel)

	for _, test := range []struct{ name, engine, host, message string }{
		{"unsupported", "unsupported-synthetic", "", "unsupported storage type:"},
		{"invalid endpoint", "minio", "invalid endpoint/path", "failed to create MinIO storage:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			values := viper.New()
			values.Set("general.storage_engine", test.engine)
			values.Set("minio.host", test.host)
			cfg := &config.Config{}
			if err := cfg.ParseGeneralConfig(values); err != nil {
				t.Fatal(err)
			}
			if test.engine == "minio" {
				if err := cfg.ParseStorageEngineConfig(values); err != nil {
					t.Fatal(err)
				}
			}
			err := Init(context.Background(), cfg)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected initialization error: %v", err)
			}
			if factory.GetStorage() != sentinel {
				t.Fatal("failed initialization replaced the existing backend")
			}
		})
	}
	if err := Init(context.Background(), nil); err == nil {
		t.Fatal("nil configuration accepted")
	}
	if factory.GetStorage() != sentinel {
		t.Fatal("nil configuration replaced the existing backend")
	}
}
