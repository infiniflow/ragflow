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
	"strings"

	"github.com/spf13/viper"
)

type DocEngineConfig struct {
	ES        ElasticsearchConfig `mapstructure:"es"`
	Infinity  InfinityConfig      `mapstructure:"infinity"`
	OceanBase OceanBaseConfig     `mapstructure:"oceanbase"`
	SeekDB    OceanBaseConfig     `mapstructure:"seekdb"`
	SereneDB  SereneDBConfig      `mapstructure:"serenedb"`
	Vastbase  VastbaseConfig      `mapstructure:"vastbase"`
}

// OceanBaseConfig mirrors the existing oceanbase/seekdb service_conf.yaml
// structure used by the Python connector.
type OceanBaseConfig struct {
	Scheme string                    `mapstructure:"scheme"`
	Config OceanBaseConnectionConfig `mapstructure:"config"`
}

// OceanBaseConnectionConfig contains the MySQL-protocol connection settings
// used by both OceanBase and SeekDB document engines.
type OceanBaseConnectionConfig struct {
	DBName         string `mapstructure:"db_name"`
	User           string `mapstructure:"user"`
	Password       string `mapstructure:"password"`
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	MaxConnections int    `mapstructure:"max_connections"`
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

// SereneDBConfig SereneDB configuration. SereneDB speaks the PostgreSQL wire
// protocol, so the engine connects with database/sql + lib/pq.
type SereneDBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"db_name"`
	// SSLMode is the lib/pq sslmode; empty defaults to "disable" for a trusted
	// local deployment. Set it (e.g. "require") to encrypt the connection.
	SSLMode string `mapstructure:"ssl_mode"`
}

// VastbaseConfig Vastbase G100 configuration. Vastbase is PostgreSQL-wire
// based (database/sql + lib/pq). DBCompatibility selects the compatibility
// mode of the instance: "PG" (pgvector-style <=> cosine distance, GIN
// to_tsvector full-text index) or "B" (Oracle-flavored <+> distance operator,
// per-column ADD INDEX USING "fulltext").
type VastbaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"db_name"`
	DBCompatibility string `mapstructure:"dbcompatibility"`
	// SSLMode is the lib/pq sslmode. Empty is not a plaintext license: the
	// engine decides by host — disable on loopback or unix sockets,
	// verify-full on every other TCP host. Plaintext to a non-loopback host
	// (e.g. the bundled docker network) must set this explicitly.
	SSLMode string `mapstructure:"ssl_mode"`
}

// ParseDocEngineConfig reads every document-engine section from the
// service configuration into the doc engine settings.
func (c *Config) ParseDocEngineConfig(v *viper.Viper) error {
	c.parseInfinityConfig(v)
	c.parseElasticsearchConfig(v)
	c.parseOceanBaseConfig(v, "oceanbase", &c.docEngine.OceanBase)
	c.parseOceanBaseConfig(v, "seekdb", &c.docEngine.SeekDB)
	c.parseSereneDBConfig(v)
	c.parseVastbaseConfig(v)
	return nil
}

func (c *Config) parseVastbaseConfig(v *viper.Viper) {
	// Default Vastbase config. SSLMode stays empty (unset): buildDSN picks
	// disable for loopback hosts and verify-full for everything else, so a
	// remote Host without an explicit ssl_mode never degrades to cleartext.
	c.docEngine.Vastbase.Host = "vastbase"
	c.docEngine.Vastbase.Port = 5432
	c.docEngine.Vastbase.User = "ragflow"
	c.docEngine.Vastbase.Password = "Infini_Rag@123"
	c.docEngine.Vastbase.DBName = "ragflow"
	c.docEngine.Vastbase.DBCompatibility = "PG"

	// Fully qualified parent keys, not v.Sub("vastbase"): Sub drops the root
	// AutomaticEnv (env prefix/replacer), and IsSet("vastbase") cannot see
	// child environment variables, so an env-only deployment
	// (RAGFLOW_VASTBASE_HOST, ... with no YAML section) would silently keep
	// every default.
	if v.IsSet("vastbase.host") {
		c.docEngine.Vastbase.Host = v.GetString("vastbase.host")
	}
	if v.IsSet("vastbase.port") {
		c.docEngine.Vastbase.Port = v.GetInt("vastbase.port")
	}
	if v.IsSet("vastbase.user") {
		c.docEngine.Vastbase.User = v.GetString("vastbase.user")
	}
	if v.IsSet("vastbase.password") {
		c.docEngine.Vastbase.Password = v.GetString("vastbase.password")
	}
	if v.IsSet("vastbase.db_name") {
		c.docEngine.Vastbase.DBName = v.GetString("vastbase.db_name")
	}
	if v.IsSet("vastbase.dbcompatibility") {
		c.docEngine.Vastbase.DBCompatibility = v.GetString("vastbase.dbcompatibility")
	}
	if v.IsSet("vastbase.ssl_mode") {
		c.docEngine.Vastbase.SSLMode = v.GetString("vastbase.ssl_mode")
	}

	c.docEngine.Vastbase.DBCompatibility = strings.ToUpper(strings.TrimSpace(c.docEngine.Vastbase.DBCompatibility))
	if c.docEngine.Vastbase.DBCompatibility != "B" {
		c.docEngine.Vastbase.DBCompatibility = "PG"
	}
}

func (c *Config) parseSereneDBConfig(v *viper.Viper) {
	// Default SereneDB config
	c.docEngine.SereneDB.Host = "localhost"
	c.docEngine.SereneDB.Port = 5432
	c.docEngine.SereneDB.User = "postgres"
	c.docEngine.SereneDB.DBName = "default_db"

	if !v.IsSet("serenedb") {
		return
	}
	sub := v.Sub("serenedb")
	if sub == nil {
		return
	}

	if sub.IsSet("host") {
		c.docEngine.SereneDB.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		c.docEngine.SereneDB.Port = sub.GetInt("port")
	}

	if sub.IsSet("user") {
		c.docEngine.SereneDB.User = sub.GetString("user")
	}

	if sub.IsSet("password") {
		c.docEngine.SereneDB.Password = sub.GetString("password")
	}

	if sub.IsSet("db_name") {
		c.docEngine.SereneDB.DBName = sub.GetString("db_name")
	}

	if sub.IsSet("ssl_mode") {
		c.docEngine.SereneDB.SSLMode = sub.GetString("ssl_mode")
	}
}

func (c *Config) parseOceanBaseConfig(v *viper.Viper, key string, target *OceanBaseConfig) {
	defaultPassword := c.database.MySQL.Password
	target.Scheme = "oceanbase"
	target.Config = OceanBaseConnectionConfig{
		DBName:         "test",
		User:           "root@test",
		Password:       defaultPassword,
		Host:           "localhost",
		Port:           2881,
		MaxConnections: 300,
	}
	if key == "seekdb" {
		target.Config.DBName = "ragflow_doc"
		target.Config.User = "root"
	}

	if !v.IsSet(key) {
		return
	}
	sub := v.Sub(key)
	if sub == nil {
		return
	}
	if sub.IsSet("scheme") {
		target.Scheme = sub.GetString("scheme")
	}
	connection := sub.Sub("config")
	if connection == nil {
		return
	}
	if connection.IsSet("db_name") {
		target.Config.DBName = connection.GetString("db_name")
	}
	if connection.IsSet("user") {
		target.Config.User = connection.GetString("user")
	}
	if connection.IsSet("password") {
		target.Config.Password = connection.GetString("password")
	}
	if connection.IsSet("host") {
		target.Config.Host = connection.GetString("host")
	}
	if connection.IsSet("port") {
		target.Config.Port = connection.GetInt("port")
	}
	if connection.IsSet("max_connections") {
		target.Config.MaxConnections = connection.GetInt("max_connections")
	}
}

// ResolveOceanBaseConnection returns the effective existing configuration for
// an OceanBase-family document engine. With scheme=mysql, Python takes the
// endpoint and credentials from the mysql section while retaining db_name from
// the nested oceanbase/seekdb config; Go intentionally follows that contract.
func (c *Config) ResolveOceanBaseConnection(engineType string) (OceanBaseConnectionConfig, error) {
	var configured OceanBaseConfig
	switch strings.ToLower(engineType) {
	case "oceanbase":
		configured = c.docEngine.OceanBase
	case "seekdb":
		configured = c.docEngine.SeekDB
	default:
		return OceanBaseConnectionConfig{}, fmt.Errorf("not an OceanBase-family engine: %s", engineType)
	}

	resolved := configured.Config
	if strings.EqualFold(configured.Scheme, "mysql") {
		mysqlConfig := c.database.MySQL
		resolved.User = mysqlConfig.User
		resolved.Password = mysqlConfig.Password
		resolved.Host = mysqlConfig.Host
		resolved.Port = mysqlConfig.Port
		resolved.MaxConnections = mysqlConfig.MaxConnections
	}
	return resolved, nil
}

func (c *Config) GetOceanBaseConfig() OceanBaseConfig {
	return c.docEngine.OceanBase
}

func (c *Config) GetSeekDBConfig() OceanBaseConfig {
	return c.docEngine.SeekDB
}

func (o OceanBaseConfig) ExportConfigs() map[string]interface{} {
	return map[string]interface{}{
		"scheme": o.Scheme,
		"config": map[string]interface{}{
			"db_name":         o.Config.DBName,
			"user":            o.Config.User,
			"password":        o.Config.Password,
			"host":            o.Config.Host,
			"port":            o.Config.Port,
			"max_connections": o.Config.MaxConnections,
		},
	}
}

func (c *Config) parseInfinityConfig(v *viper.Viper) {
	// Default Infinity config
	c.docEngine.Infinity.URI = "localhost:23817"
	c.docEngine.Infinity.PostgresPort = 5432
	c.docEngine.Infinity.DBName = "default_db"
	c.docEngine.Infinity.MappingFileName = "infinity_mapping.json"
	c.docEngine.Infinity.DocMetaMappingFileName = "doc_meta_infinity_mapping.json"

	if !v.IsSet("infinity") {
		return
	}
	sub := v.Sub("infinity")
	if sub == nil {
		return
	}

	if sub.IsSet("uri") {
		c.docEngine.Infinity.URI = sub.GetString("uri")
	}

	if sub.IsSet("postgres_port") {
		c.docEngine.Infinity.PostgresPort = sub.GetInt("postgres_port")
	}

	if sub.IsSet("db_name") {
		c.docEngine.Infinity.DBName = sub.GetString("db_name")
	}

	if sub.IsSet("mapping_file_name") {
		c.docEngine.Infinity.MappingFileName = sub.GetString("mapping_file_name")
	}

	if sub.IsSet("doc_meta_mapping_file_name") {
		c.docEngine.Infinity.DocMetaMappingFileName = sub.GetString("doc_meta_mapping_file_name")
	}
}

func (c *Config) parseElasticsearchConfig(v *viper.Viper) {
	// Default Elasticsearch config
	c.docEngine.ES.Hosts = "http://localhost:1200"
	c.docEngine.ES.Username = "elastic"
	c.docEngine.ES.Password = "infini_rag_flow"

	if !v.IsSet("es") {
		return
	}
	sub := v.Sub("es")
	if sub == nil {
		return
	}

	if sub.IsSet("hosts") {
		c.docEngine.ES.Hosts = sub.GetString("hosts")
	}

	if sub.IsSet("username") {
		c.docEngine.ES.Username = sub.GetString("username")
	}

	if sub.IsSet("password") {
		c.docEngine.ES.Password = sub.GetString("password")
	}
}

func (c *Config) GetElasticsearchConfig() ElasticsearchConfig {
	return c.docEngine.ES
}

func (e ElasticsearchConfig) ExportConfigs() map[string]interface{} {
	var esConfigs map[string]interface{}
	esConfigs = make(map[string]interface{})
	esConfigs["hosts"] = e.Hosts
	esConfigs["username"] = e.Username
	esConfigs["password"] = e.Password
	return esConfigs
}

func (c *Config) IsElasticConfigured() bool {
	return c.docEngine.ES.Hosts != ""
}

func (c *Config) GetInfinityConfig() InfinityConfig {
	return c.docEngine.Infinity
}

func (i InfinityConfig) ExportConfigs() map[string]interface{} {
	var infinityConfigs map[string]interface{}
	infinityConfigs = make(map[string]interface{})
	infinityConfigs["uri"] = i.URI
	infinityConfigs["postgres_port"] = i.PostgresPort
	infinityConfigs["db_name"] = i.DBName
	infinityConfigs["mapping_file_name"] = i.MappingFileName
	infinityConfigs["doc_meta_mapping_file_name"] = i.DocMetaMappingFileName
	return infinityConfigs
}

// GetSereneDBConfig returns the SereneDB doc engine connection settings.
func (c *Config) GetSereneDBConfig() SereneDBConfig {
	return c.docEngine.SereneDB
}

func (c *Config) GetVastbaseConfig() VastbaseConfig {
	return c.docEngine.Vastbase
}

func (v VastbaseConfig) ExportConfigs() map[string]interface{} {
	return map[string]interface{}{
		"host":            v.Host,
		"port":            v.Port,
		"user":            v.User,
		"password":        v.Password,
		"db_name":         v.DBName,
		"dbcompatibility": v.DBCompatibility,
		"ssl_mode":        v.SSLMode,
	}
}
