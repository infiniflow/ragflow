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

// ParseServerNames reads each service identity from <service>.name or
// RAGFLOW_<SERVICE>_NAME. Reading full keys also supports env-only settings.
func (c *Config) ParseServerNames(v *viper.Viper) {
	c.serverNames = make(map[string]string)
	for _, mode := range []string{"admin", "api", "ingestor", "syncer", "chatter", "deepdoc"} {
		if v.IsSet(mode + ".name") {
			c.serverNames[mode] = v.GetString(mode + ".name")
		}
	}
}

func (c *Config) GetServerName(mode string) *string {
	if name, ok := c.serverNames[mode]; ok {
		return &name
	}
	return nil
}
