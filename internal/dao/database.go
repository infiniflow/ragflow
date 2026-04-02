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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"ragflow/internal/entity"
	"strings"
	"time"

	"ragflow/internal/logger"

	"ragflow/internal/server"
	"ragflow/internal/utility"

	"go.uber.org/zap"
	gormLogger "gorm.io/gorm/logger"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB
var modelProviderManager *entity.ProviderManager

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

	// Auto migrate all models
	models := []interface{}{
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
	}

	for _, m := range models {
		if err = autoMigrateSafely(DB, m); err != nil {
			return fmt.Errorf("failed to migrate model %T: %w", m, err)
		}
	}

	// Run manual migrations for complex schema changes
	if err = RunMigrations(DB); err != nil {
		return fmt.Errorf("failed to run manual migrations: %w", err)
	}

	logger.Info("Database connected and migrated successfully")

	modelProviderManager, err = entity.NewProviderManager("conf/models")
	if err != nil {
		log.Fatal("Failed to load model providers:", err)
	}
	logger.Info("Model providers loaded successfully")
	return nil
}

// GetDB get database instance
func GetDB() *gorm.DB {
	return DB
}

// GetModelProviderManager get database instance
func GetModelProviderManager() *entity.ProviderManager {
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
		logger.Info("Index already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1060") && strings.Contains(errStr, "Duplicate column name") {
		logger.Info("Column already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1050") && strings.Contains(errStr, "Table") {
		logger.Info("Table already exists, skipping", zap.String("error", errStr))
		return nil
	}

	return err
}

// InitLLMFactory initializes LLM factories and models from JSON file.
// It reads the llm_factories.json configuration file and populates the database
// with LLM factory and model information. If a factory or model already exists,
// it will be updated with the new configuration.
//
// Returns:
//   - error: An error if the initialization fails, nil otherwise.
func InitLLMFactory() error {
	configPath := filepath.Join(utility.GetProjectBaseDirectory(), "conf", "llm_factories.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read llm_factories.json: %w", err)
	}

	var fileData LLMFactoriesFile
	if err := json.Unmarshal(data, &fileData); err != nil {
		return fmt.Errorf("failed to parse llm_factories.json: %w", err)
	}

	db := DB

	for _, factory := range fileData.FactoryLLMInfos {
		status := factory.Status
		if status == "" {
			status = "1"
		}

		llmFactory := &entity.LLMFactories{
			Name:   factory.Name,
			Logo:   utility.StringPtr(factory.Logo),
			Tags:   factory.Tags,
			Rank:   utility.ParseInt64(factory.Rank),
			Status: &status,
		}

		var existingFactory entity.LLMFactories
		result := db.Where("name = ?", factory.Name).First(&existingFactory)
		if result.Error != nil {
			if err := db.Create(llmFactory).Error; err != nil {
				log.Printf("Failed to create LLM factory %s: %v", factory.Name, err)
				continue
			}
		} else {
			if err := db.Model(&entity.LLMFactories{}).Where("name = ?", factory.Name).Updates(map[string]interface{}{
				"logo":   llmFactory.Logo,
				"tags":   llmFactory.Tags,
				"rank":   llmFactory.Rank,
				"status": llmFactory.Status,
			}).Error; err != nil {
				log.Printf("Failed to update LLM factory %s: %v", factory.Name, err)
			}
		}

		for _, llm := range factory.LLM {
			llmStatus := "1"
			llmModel := &entity.LLM{
				LLMName:   llm.LLMName,
				ModelType: llm.ModelType,
				FID:       factory.Name,
				MaxTokens: llm.MaxTokens,
				Tags:      llm.Tags,
				IsTools:   llm.IsTools,
				Status:    &llmStatus,
			}

			var existingLLM entity.LLM
			result := db.Where("llm_name = ? AND fid = ?", llm.LLMName, factory.Name).First(&existingLLM)
			if result.Error != nil {
				if err := db.Create(llmModel).Error; err != nil {
					log.Printf("Failed to create LLM %s/%s: %v", factory.Name, llm.LLMName, err)
				}
			} else {
				if err := db.Model(&entity.LLM{}).Where("llm_name = ? AND fid = ?", llm.LLMName, factory.Name).Updates(map[string]interface{}{
					"model_type": llmModel.ModelType,
					"max_tokens": llmModel.MaxTokens,
					"tags":       llmModel.Tags,
					"is_tools":   llmModel.IsTools,
					"status":     llmModel.Status,
				}).Error; err != nil {
					log.Printf("Failed to update LLM %s/%s: %v", factory.Name, llm.LLMName, err)
				}
			}
		}
	}

	return nil
}
