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
	"net/mail"
	"ragflow/internal/common"
	"strings"
)

type Environments struct {
	Language string `mapstructure:"language"`

	SecretKey string `mapstructure:"secret_key"`

	EnableRegister       *bool `mapstructure:"enable_register"`
	DisablePasswordLogin *bool `mapstructure:"disable_password_login"`

	DocumentEngineType string `mapstructure:"document_engine_type"`
	DatabaseType       string `mapstructure:"database_type"`

	StorageType string `mapstructure:"storage_type"`
	MinioHost   string `mapstructure:"minio_host"`
	MinioRegion string `mapstructure:"minio_region"`

	CreateDefaultSuperUser bool             `mapstructure:"create_default_super_user"`
	DefaultSuperUser       DefaultSuperUser `mapstructure:"default_super_user"`
}

type DefaultSuperUser struct {
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
	Nickname string `mapstructure:"nickname"`
}

func (c *Config) GetEnvironments() error {
	// Secret key
	if envVal := common.GetEnv(common.EnvRAGFlowSecretKey); envVal != "" {
		c.environments.SecretKey = envVal
	}

	if envVal := common.GetEnv(common.EnvEnableRegister); envVal != "" {
		if c.environments.EnableRegister == nil {
			c.environments.EnableRegister = new(bool)
		}
		str := strings.ToLower(envVal)
		if str == "true" || str == "1" || str == "yes" {
			*c.environments.EnableRegister = true
		} else {
			*c.environments.EnableRegister = false
		}
	}

	if envVal := common.GetEnv(common.EnvDisablePasswordLogin); envVal != "" {
		if c.environments.DisablePasswordLogin == nil {
			c.environments.DisablePasswordLogin = new(bool)
		}
		str := strings.ToLower(envVal)
		if str == "true" || str == "1" || str == "yes" {
			*c.environments.DisablePasswordLogin = true
		} else {
			*c.environments.DisablePasswordLogin = false
		}
	}

	// Doc engine
	docEngine := common.GetEnvSmall(common.EnvDocEngine)
	if docEngine != "" {
		switch docEngine {
		case "infinity", "elasticsearch":
			c.environments.DocumentEngineType = docEngine
		case "opensearch", "oceanbase":
			return fmt.Errorf("not implemented: %s", docEngine)
		default:
			return fmt.Errorf("invalid doc engine: %s", docEngine)
		}
	}

	// Default super user email
	superUserEmail := common.GetEnv(common.EnvDefaultSuperuserEmail)
	if superUserEmail != "" {
		_, err := mail.ParseAddress(superUserEmail)
		if err != nil {
			return fmt.Errorf("invalid super user email: %s", superUserEmail)
		}
		c.environments.DefaultSuperUser.Email = superUserEmail
	}

	superUserPassword := common.GetEnv(common.EnvDefaultSuperuserPassword)
	if superUserPassword != "" {
		c.environments.DefaultSuperUser.Password = superUserPassword
	}

	superUserNickname := common.GetEnv(common.EnvDefaultSuperuserNickname)
	if superUserNickname != "" {
		c.environments.DefaultSuperUser.Nickname = superUserNickname
	}

	// Meta database
	databaseType := common.GetEnvSmall(common.EnvDBType)
	if databaseType != "" {
		switch databaseType {
		case "mysql":
			c.environments.DatabaseType = "mysql"
		default:
			return fmt.Errorf("invalid database type: %s", databaseType)
		}
	}

	// Storage
	storageType := common.GetEnvSmall(common.EnvStorageImpl)
	if storageType != "" {
		switch storageType {
		case "minio", "s3", "oss", "gcs":
			c.environments.StorageType = storageType
		default:
			return fmt.Errorf("invalid storage type: %s", storageType)
		}
	}

	// Minio
	minioHost := strings.ToLower(common.GetEnv(common.EnvMinioHost))
	if minioHost != "" {
		c.environments.MinioHost = minioHost
	}

	minioRegion := strings.ToLower(common.GetEnv(common.EnvMinioRegion))
	if minioRegion != "" {
		c.environments.MinioRegion = minioRegion
	}

	c.environments.Language = GetLanguage()

	return nil
}

func GetLanguage() string {
	lang := common.GetEnv(common.EnvLang)
	if lang == "" {
		lang = common.GetEnv(common.EnvLanguage)
	}

	lang = strings.ToLower(lang)

	if strings.Contains(lang, "zh_") ||
		strings.Contains(lang, "zh-") ||
		strings.HasPrefix(lang, "zh") {
		return "Chinese"
	}

	return "English"
}

func (c *Config) GetSecretKey() string {
	return c.environments.SecretKey
}
