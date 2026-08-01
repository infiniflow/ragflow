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

package server

import (
	"errors"
	"fmt"
	"net/url"
	"ragflow/internal/server/config"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// DefaultConnectTimeout default connection timeout for external services
const DefaultConnectTimeout = 5 * time.Second

// OAuthConfig OAuth configuration for a channel.
// Mirrors api/apps/auth/__init__.py's OAUTH_CONFIG entries: a Type that
// selects the auth client flavor (oauth2 / oidc / GitHub), plus the
// transport URLs and client credentials. For OIDC the URLs are derived
// from Issuer via the .well-known/openid-configuration document, so they
// may be left blank.
type OAuthConfig struct {
	DisplayName      string `mapstructure:"display_name"`
	Icon             string `mapstructure:"icon"`
	Type             string `mapstructure:"type"`
	ClientID         string `mapstructure:"client_id"`
	ClientSecret     string `mapstructure:"client_secret"`
	AuthorizationURL string `mapstructure:"authorization_url"`
	TokenURL         string `mapstructure:"token_url"`
	UserinfoURL      string `mapstructure:"userinfo_url"`
	RedirectURI      string `mapstructure:"redirect_uri"`
	Scope            string `mapstructure:"scope"`
	Issuer           string `mapstructure:"issuer"`
}

var (
	globalConfig *config.Config
	globalViper  *viper.Viper
	zapLogger    *zap.Logger
)

// Init initialize configuration
func Init(configPath string) error {

	v := viper.New()

	// Set configuration file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Try to load service_conf.yaml from conf directory first
		v.SetConfigName("service_conf")
		v.SetConfigType("yaml")
		v.AddConfigPath("./conf")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/ragflow/")
	}

	// Read environment variables
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return fmt.Errorf("read config file error: %w", err)
		}
		zapLogger.Info("Config file not found, using environment variables only")
	}

	// Save viper instance
	globalViper = v

	globalConfig = &config.Config{}
	err := globalConfig.ParseGeneralConfig(v)
	if err != nil {
		return fmt.Errorf("parse general config error: %w", err)
	}

	err = globalConfig.ParseDatabaseConfig(v)
	if err != nil {
		return fmt.Errorf("parse database config error: %w", err)
	}

	err = globalConfig.ParseDocEngineConfig(v)
	if err != nil {
		return fmt.Errorf("parse doc engine config error: %w", err)
	}

	err = globalConfig.ParseStorageEngineConfig(v)
	if err != nil {
		return fmt.Errorf("parse storage engine config error: %w", err)
	}

	err = globalConfig.ParseCacheEngineConfig(v)
	if err != nil {
		return fmt.Errorf("parse cache engine config error: %w", err)
	}

	err = globalConfig.ParseQueueEngineConfig(v)
	if err != nil {
		return fmt.Errorf("parse queue engine config error: %w", err)
	}

	err = globalConfig.ParseAnalyticEngineConfig(v)
	if err != nil {
		return fmt.Errorf("parse analytic engine config error: %w", err)
	}

	err = globalConfig.ParseOpenTelemetryConfig(v)
	if err != nil {
		return fmt.Errorf("parse open telemetry config error: %w", err)
	}

	err = globalConfig.ParseAdminConfig(v)
	if err != nil {
		return fmt.Errorf("parse admin config error: %w", err)
	}

	err = globalConfig.ParseAPIServerConfig(v)
	if err != nil {
		return fmt.Errorf("parse API server config error: %w", err)
	}

	err = globalConfig.ParseIngestorConfig(v)
	if err != nil {
		return fmt.Errorf("parse ingestor config error: %w", err)
	}

	err = globalConfig.ParseSyncerConfig(v)
	if err != nil {
		return fmt.Errorf("parse syncer config error: %w", err)
	}

	err = globalConfig.ParseLogConfig(v)
	if err != nil {
		return fmt.Errorf("parse log config error: %w", err)
	}

	err = globalConfig.ParseSMTPConfig(v)
	if err != nil {
		return fmt.Errorf("parse SMTP config error: %w", err)
	}

	err = globalConfig.GetEnvironments()
	if err != nil {
		return fmt.Errorf("get environments error: %w", err)
	}

	err = globalConfig.ParseBillingConfig(v)
	if err != nil {
		return fmt.Errorf("parse billing config error: %w", err)
	}

	err = globalConfig.ParseDefaultModelsConfig(v)
	if err != nil {
		return fmt.Errorf("parse default models config error: %w", err)
	}

	err = globalConfig.ParseOAuthConfig(v)
	if err != nil {
		return fmt.Errorf("parse OAuth config error: %w", err)
	}

	return nil
}

// GetConfig gets the global configuration
func GetConfig() *config.Config {
	return globalConfig
}

// SetLogger sets the logger instance
func SetLogger(l *zap.Logger) {
	zapLogger = l
}

func GetAllConfigs() ([]map[string]interface{}, error) {
	var allConfigs []map[string]interface{}

	// Database
	databaseType := globalConfig.DatabaseType()
	switch databaseType {
	case "mysql":
		mysqlConfig := globalConfig.GetMySQLConfig()
		exportedMySQLConfigs := mysqlConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedMySQLConfigs)
	default:
		return nil, fmt.Errorf("not supported database: %s", databaseType)
	}

	// Doc engine
	docEngineType := globalConfig.DocEngineType()
	switch docEngineType {
	case "elasticsearch":
		elasticConfig := globalConfig.GetElasticsearchConfig()
		exportedESConfigs := elasticConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedESConfigs)
	case "infinity":
		infinityConfig := globalConfig.GetInfinityConfig()
		exportedInfinityConfigs := infinityConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedInfinityConfigs)
	default:
		return nil, fmt.Errorf("not supported doc engine: %s", docEngineType)
	}

	// storage engine
	storageType := globalConfig.StorageEngineType()
	switch storageType {
	case "minio":
		minioConfig := globalConfig.GetMinioConfig()
		exportedMinioConfigs := minioConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedMinioConfigs)
	case "s3":
		s3Config := globalConfig.GetS3Config()
		exportedS3Configs := s3Config.ExportConfigs()
		allConfigs = append(allConfigs, exportedS3Configs)
	case "oss":
		ossConfig := globalConfig.GetOSSConfig()
		exportedOSSConfigs := ossConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedOSSConfigs)
	case "gcs":
		gcsConfig := globalConfig.GetGCSConfig()
		exportedGCSConfigs := gcsConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedGCSConfigs)
	default:
		return nil, fmt.Errorf("not supported storage engine: %s", storageType)
	}

	// cache engine
	cacheType := globalConfig.CacheEngineType()
	switch cacheType {
	case "redis":
		redisConfig := globalConfig.GetRedisConfig()
		exportedRedisConfigs := redisConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedRedisConfigs)
	default:
		return nil, fmt.Errorf("not supported cache engine: %s", cacheType)
	}

	// message queue
	messageQueueType := globalConfig.QueueEngineType()
	switch messageQueueType {
	case "nats":
		natsConfig := globalConfig.GetNATSConfig()
		exportedNatsConfigs := natsConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedNatsConfigs)
	default:
		return nil, fmt.Errorf("not supported message queue: %s", messageQueueType)
	}

	// analytical engine
	olapType := globalConfig.AnalyticEngineType()
	switch olapType {
	case "clickhouse":
		clickhouseConfig := globalConfig.GetClickhouseConfig()
		exportedClickhouseConfigs := clickhouseConfig.ExportConfigs()
		allConfigs = append(allConfigs, exportedClickhouseConfigs)
	default:
		return nil, fmt.Errorf("not supported analytical engine: %s", olapType)
	}

	// tracing engine
	oTelConfig := globalConfig.GetOpenTelemetryConfig()
	exportedOTELConfigs := oTelConfig.ExportConfigs()
	allConfigs = append(allConfigs, exportedOTELConfigs)

	return allConfigs, nil
}

// PrintAll prints all configuration settings
func PrintAll() {
	if globalViper == nil {
		zapLogger.Info("Configuration not initialized")
		return
	}

	allSettings := globalViper.AllSettings()
	zapLogger.Info("=== All Configurations ===")
	for key, value := range allSettings {
		zapLogger.Info("config", zap.String("key", key), zap.Any("value", value))
	}
	zapLogger.Info("=== End Configurations ===")
}

// parseHostPort parses host:port string and returns host and port
func parseHostPort(hostPort string) (string, int) {
	if hostPort == "" {
		return "", 0
	}

	// Handle URL format like http://host:port
	if strings.Contains(hostPort, "://") {
		u, err := url.Parse(hostPort)
		if err == nil {
			hostPort = u.Host
		}
	}

	// Split host:port
	parts := strings.Split(hostPort, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		port, _ = strconv.Atoi(parts[1])
	}
	return host, port
}
