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

// OpenGaussConfig configures the DataVec document database.
type OpenGaussConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	Schema   string
	SSLMode  string `mapstructure:"ssl_mode"`
}

func (c *Config) GetOpenGaussConfig() OpenGaussConfig { return c.docEngine.OpenGauss }

func (c *Config) parseOpenGaussConfig(v *viper.Viper) {
	// Read the password directly so Docker never interpolates credentials into YAML.
	v.MustBindEnv("opengauss.password", "RAGFLOW_OPENGAUSS_PASSWORD", "OPENGAUSS_PASSWORD")
	v.SetDefault("opengauss.host", "localhost")
	v.SetDefault("opengauss.port", 5432)
	v.SetDefault("opengauss.user", "ragflow")
	v.SetDefault("opengauss.database", "postgres")
	v.SetDefault("opengauss.schema", "public")
	c.docEngine.OpenGauss = OpenGaussConfig{
		Host: v.GetString("opengauss.host"), Port: v.GetInt("opengauss.port"),
		User: v.GetString("opengauss.user"), Password: v.GetString("opengauss.password"),
		Database: v.GetString("opengauss.database"), Schema: v.GetString("opengauss.schema"),
		SSLMode: v.GetString("opengauss.ssl_mode"),
	}
}

// ExportConfigs returns the connection details exposed by the system configuration API.
func (c OpenGaussConfig) ExportConfigs() map[string]interface{} {
	return map[string]interface{}{"host": c.Host, "port": c.Port, "database": c.Database, "schema": c.Schema, "user": c.User, "ssl_mode": c.SSLMode}
}
