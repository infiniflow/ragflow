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

package init_data

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// LLMFactoryConfig represents a single LLM factory configuration
type LLMFactoryConfig struct {
	Name   string        `json:"name"`
	Logo   string        `json:"logo"`
	Tags   string        `json:"tags"`
	Status string        `json:"status"`
	Rank   string        `json:"rank"`
	LLM    []LLMConfig   `json:"llm"`
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

// InitLLMFactory initializes LLM factories and models from JSON file
func InitLLMFactory() error {
	configPath := filepath.Join(getProjectBaseDirectory(), "conf", "llm_factories.json")
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read llm_factories.json: %w", err)
	}

	var fileData LLMFactoriesFile
	if err := json.Unmarshal(data, &fileData); err != nil {
		return fmt.Errorf("failed to parse llm_factories.json: %w", err)
	}

	db := dao.DB

	for _, factory := range fileData.FactoryLLMInfos {
		status := factory.Status
		if status == "" {
			status = "1"
		}

		llmFactory := &model.LLMFactories{
			Name:   factory.Name,
			Logo:   stringPtr(factory.Logo),
			Tags:   factory.Tags,
			Rank:   parseInt64(factory.Rank),
			Status: &status,
		}

		var existingFactory model.LLMFactories
		result := db.Where("name = ?", factory.Name).First(&existingFactory)
		if result.Error != nil {
			if err := db.Create(llmFactory).Error; err != nil {
				log.Printf("Failed to create LLM factory %s: %v", factory.Name, err)
				continue
			}
		} else {
			if err := db.Model(&model.LLMFactories{}).Where("name = ?", factory.Name).Updates(map[string]interface{}{
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
			llmModel := &model.LLM{
				LLMName:   llm.LLMName,
				ModelType: llm.ModelType,
				FID:       factory.Name,
				MaxTokens: llm.MaxTokens,
				Tags:      llm.Tags,
				IsTools:   llm.IsTools,
				Status:    &llmStatus,
			}

			var existingLLM model.LLM
			result := db.Where("llm_name = ? AND fid = ?", llm.LLMName, factory.Name).First(&existingLLM)
			if result.Error != nil {
				if err := db.Create(llmModel).Error; err != nil {
					log.Printf("Failed to create LLM %s/%s: %v", factory.Name, llm.LLMName, err)
				}
			} else {
				if err := db.Model(&model.LLM{}).Where("llm_name = ? AND fid = ?", llm.LLMName, factory.Name).Updates(map[string]interface{}{
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

	log.Println("LLM factories initialized successfully")
	return nil
}

func getProjectBaseDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseInt64(s string) int64 {
	var result int64
	fmt.Sscanf(s, "%d", &result)
	return result
}
