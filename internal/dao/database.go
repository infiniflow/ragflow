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

package dao

import (
	"fmt"
	"log"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"strings"
	"time"

	"ragflow/internal/server"

	"go.uber.org/zap"
	gormLogger "gorm.io/gorm/logger"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB
var modelProviderManager *models.ProviderManager

// LLMFactoryConfig represents a single LLM factory configuration
type LLMFactoryConfig struct {
	Name   string      `json:"name"`
	Logo   string      `json:"logo"`
	Tags   string      `json:"tags"`
	Status string      `json:"status"`
	Rank   string      `json:"rank"`
	LLM    []LLMConfig `json:"llm"`
}

// LLMConfig represents a single LLM model configuration
type LLMConfig struct {
	LLMName   string `json:"llm_name"`
	Tags      string `json:"tags"`
	MaxTokens int64  `json:"max_tokens"`
	ModelType string `json:"model_type"`
	IsTools   bool   `json:"is_tools"`
}

// LLMFactoriesFile represents the structure of llm_factories.json
type LLMFactoriesFile struct {
	FactoryLLMInfos []LLMFactoryConfig `json:"factory_llm_infos"`
}

// InitDB initialize database connection
func InitDB() error {
	cfg := server.GetConfig()
	dbCfg := cfg.Database

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		dbCfg.Username,
		dbCfg.Password,
		dbCfg.Host,
		dbCfg.Port,
		dbCfg.Database,
		dbCfg.Charset,
	)

	// Set log level
	var gormLogLevel gormLogger.LogLevel
	if cfg.Server.Mode == "debug" {
		gormLogLevel = gormLogger.Info
	} else {
		gormLogLevel = gormLogger.Silent
	}

	// Connect to database
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogLevel),
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
		TranslateError: true,
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// Get general database object sql.DB
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Auto migrate all dataModels
	dataModels := []interface{}{
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.File{},
		&entity.File2Document{},
		&entity.TenantLLM{},
		&entity.Chat{},
		&entity.ChatSession{},
		&entity.Task{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.Knowledgebase{},
		&entity.InvitationCode{},
		&entity.Document{},
		&entity.UserCanvas{},
		&entity.CanvasTemplate{},
		&entity.UserCanvasVersion{},
		&entity.LLMFactories{},
		&entity.LLM{},
		&entity.TenantLangfuse{},
		&entity.SystemSettings{},
		&entity.Connector{},
		&entity.Connector2Kb{},
		&entity.SyncLogs{},
		&entity.MCPServer{},
		&entity.Memory{},
		&entity.Search{},
		&entity.PipelineOperationLog{},
		&entity.EvaluationDataset{},
		&entity.EvaluationCase{},
		&entity.EvaluationRun{},
		&entity.EvaluationResult{},
		&entity.TimeRecord{},
		&entity.License{},
		&entity.SkillSearchConfig{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.TenantModelGroupMapping{},
		&entity.TenantModelProvider{},
		&entity.TenantModelGroup{},
	}

	for _, m := range dataModels {
		if err = autoMigrateSafely(DB, m); err != nil {
			return fmt.Errorf("failed to migrate model %T: %w", m, err)
		}
	}

	// Run manual migrations for complex schema changes
	if err = RunMigrations(DB); err != nil {
		return fmt.Errorf("failed to run manual migrations: %w", err)
	}

	common.Info("Database connected and migrated successfully")

	err = models.InitProviderManager("conf/models")
	if err != nil {
		log.Fatal("Failed to load model providers:", err)
	}

	modelProviderManager = models.GetProviderManager()
	common.Info("Model providers loaded successfully")

	return nil
}

// GetDB get database instance
func GetDB() *gorm.DB {
	return DB
}

// GetModelProviderManager get database instance
func GetModelProviderManager() *models.ProviderManager {
	return modelProviderManager
}

// autoMigrateSafely runs AutoMigrate and ignores duplicate index errors
// This handles cases where indexes already exist (e.g., created by Python backend)
func autoMigrateSafely(db *gorm.DB, model interface{}) error {
	err := db.AutoMigrate(model)
	if err == nil {
		return nil
	}

	// Check if error is MySQL duplicate index error (Error 1061)
	errStr := err.Error()
	if strings.Contains(errStr, "Error 1061") && strings.Contains(errStr, "Duplicate key name") {
		common.Info("Index already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1060") && strings.Contains(errStr, "Duplicate column name") {
		common.Info("Column already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1050") && strings.Contains(errStr, "Table") {
		common.Info("Table already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1091") && strings.Contains(errStr, "Can't DROP") {
		common.Info("Index/column already dropped, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1138") && strings.Contains(errStr, "Invalid use of NULL") {
		common.Info("NULL value in existing rows, skipping migration change", zap.String("error", errStr))
		return nil
	}

	return err
}
