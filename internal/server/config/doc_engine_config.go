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
)

type DocEngineConfig struct {
	EngineType string              `mapstructure:"type"`
	ES         ElasticsearchConfig `mapstructure:"es"`
	Infinity   InfinityConfig      `mapstructure:"infinity"`
}

// ElasticsearchConfig Elasticsearch configuration
type ElasticsearchConfig struct {
	Hosts    string `mapstructure:"hosts"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// InfinityConfig Infinity configuration
type InfinityConfig struct {
	URI                    string `mapstructure:"uri"`
	PostgresPort           int    `mapstructure:"postgres_port"`
	DBName                 string `mapstructure:"db_name"`
	MappingFileName        string `mapstructure:"mapping_file_name"`
	DocMetaMappingFileName string `mapstructure:"doc_meta_mapping_file_name"`
}

func ParseDocEngineConfig(docEngineType string, config *Config, v *viper.Viper) error {
	switch docEngineType {
	case "infinity":
		parseInfinityConfig(config, v)
	case "es", "elasticsearch":
		parseElasticsearchConfig(config, v)
	default:
		return fmt.Errorf("doc engine type %s is not supported", docEngineType)
	}
	config.DocEngine.EngineType = docEngineType
	return nil
}

func parseInfinityConfig(config *Config, v *viper.Viper) {
	// Default Infinity config
	config.DocEngine.Infinity.URI = "localhost:23817"
	config.DocEngine.Infinity.PostgresPort = 5432
	config.DocEngine.Infinity.DBName = "default_db"
	config.DocEngine.Infinity.MappingFileName = "infinity_mapping.json"
	config.DocEngine.Infinity.DocMetaMappingFileName = "doc_meta_infinity_mapping.json"

	if !v.IsSet("infinity") {
		return
	}
	sub := v.Sub("infinity")
	if sub == nil {
		return
	}

	if sub.IsSet("uri") {
		config.DocEngine.Infinity.URI = sub.GetString("uri")
	}

	if sub.IsSet("postgres_port") {
		config.DocEngine.Infinity.PostgresPort = sub.GetInt("postgres_port")
	}

	if sub.IsSet("db_name") {
		config.DocEngine.Infinity.DBName = sub.GetString("db_name")
	}

	if sub.IsSet("mapping_file_name") {
		config.DocEngine.Infinity.MappingFileName = sub.GetString("mapping_file_name")
	}

	if sub.IsSet("doc_meta_mapping_file_name") {
		config.DocEngine.Infinity.DocMetaMappingFileName = sub.GetString("doc_meta_mapping_file_name")
	}
}

func parseElasticsearchConfig(config *Config, v *viper.Viper) {
	// Default Elasticsearch config
	config.DocEngine.ES.Hosts = "http://localhost:1200"
	config.DocEngine.ES.Username = "elastic"
	config.DocEngine.ES.Password = "infini_rag_flow"

	if !v.IsSet("es") {
		return
	}
	sub := v.Sub("es")
	if sub == nil {
		return
	}

	if sub.IsSet("hosts") {
		config.DocEngine.ES.Hosts = sub.GetString("hosts")
	}

	if sub.IsSet("username") {
		config.DocEngine.ES.Username = sub.GetString("username")
	}

	if sub.IsSet("password") {
		config.DocEngine.ES.Password = sub.GetString("password")
	}
}
