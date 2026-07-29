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

func parseDatabaseConfig(databaseType string, config *Config, v *viper.Viper) {
	switch databaseType {
	case "mysql":
		parseMySQLConfig(config, v)
	default:
		return
	}

}

//name: 'rag_flow'
//user: 'root'
//password: 'infini_rag_flow'
//host: 'localhost'
//port: 3306
//max_connections: 900
//stale_timeout: 300
//max_allowed_packet: 1073741824

func parseMySQLConfig(config *Config, v *viper.Viper) {

	config.Database

	if !v.IsSet("mysql") || config.Database.Host != "" {
		return
	}
	sub := v.Sub("mysql")
	if sub == nil {
		return
	}
	config.Database = DatabaseConfig{
		Driver:   "mysql",
		Host:     sub.GetString("host"),
		Port:     sub.GetInt("port"),
		Database: sub.GetString("name"),
		Username: sub.GetString("user"),
		Password: sub.GetString("password"),
		Charset:  "utf8mb4",
	}
}
