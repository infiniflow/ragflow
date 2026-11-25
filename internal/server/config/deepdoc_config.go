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

type DeepDocConfig struct {
	Name string `mapstructure:"name"`
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

func (c *Config) ParseDeepDocConfig(v *viper.Viper) error {
	c.deepdoc.Name = "deepdoc"
	c.deepdoc.Host = "localhost"
	c.deepdoc.Port = 9388

	if v.IsSet("deepdoc.name") {
		c.deepdoc.Name = v.GetString("deepdoc.name")
	}

	if v.IsSet("deepdoc.host") {
		c.deepdoc.Host = v.GetString("deepdoc.host")
	}

	if v.IsSet("deepdoc.port") {
		c.deepdoc.Port = v.GetInt("deepdoc.port")
	}

	return nil
}

func (c *Config) GetDeepDocServerConfig() DeepDocConfig {
	return c.deepdoc
}

func (c *Config) SetDeepDocServerPort(port int) {
	c.deepdoc.Port = port
}
