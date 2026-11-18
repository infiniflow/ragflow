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
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestServerNames(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader("ingestor:\n  name: worker-config\n")); err != nil {
		t.Fatal(err)
	}
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	t.Setenv("RAGFLOW_INGESTOR_NAME", "worker-env")
	t.Setenv("RAGFLOW_SYNCER_NAME", "sync-env")
	c := &Config{}
	c.ParseServerNames(v)
	for mode, want := range map[string]string{"ingestor": "worker-env", "syncer": "sync-env"} {
		if got := c.GetServerName(mode); got == nil || *got != want {
			t.Fatalf("%s name=%v, want %s", mode, got, want)
		}
	}
	if c.GetServerName("api") != nil {
		t.Fatal("unconfigured name must use the generated default")
	}
	t.Setenv("RAGFLOW_INGESTOR_NAME", "")
	c.ParseServerNames(v)
	if got := c.GetServerName("ingestor"); got == nil || *got != "worker-config" {
		t.Fatalf("config name=%v, want worker-config", got)
	}
}
