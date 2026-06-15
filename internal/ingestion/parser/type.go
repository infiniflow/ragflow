//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"fmt"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/utility"
)

func GetParser(fileType utility.FileType, config *ParserConfig) (FileParser, error) {
	switch fileType {
	case utility.FileTypePPTX:
		return NewPPTXParser(config)
	case utility.FileTypePPT:
		return NewPPTParser(config)
	case utility.FileTypeXLSX:
		return NewXLSXParser(config)
	case utility.FileTypeXLS:
		return NewXLSParser(config)
	case utility.FileTypeDOCX:
		return NewDOCXParser(config)
	case utility.FileTypeDOC:
		return NewDOCParser(config)
	case utility.FileTypePDF:
		return NewPDFParser(config)
	case utility.FileTypeHTML:
		config.LibType = Official
		return NewHTMLParser(config)
	case utility.FileTypeMarkdown:
		config.LibType = GoMarkdown
		return NewMarkdownParser(config)
	default:
		return nil, fmt.Errorf("unsupported file type: %s", fileType)
	}
}

// FileParser defines the interface for all file parsers.
type FileParser interface {
	// Parse parses the input text.
	Parse(filename string, data []byte) error

	String() string
}

type ParserConfig struct {
	LibType string

	ProviderEntity *entity.TenantModelProvider
	ProviderInfo   *models.Provider
	InstanceEntity *entity.TenantModelInstance

	ChatModelEntity *entity.TenantModel
	ChatModelInfo   *models.Model
	ChatAPIConfig   *models.APIConfig

	VisionModelEntity *entity.TenantModel
	VisionModelInfo   *models.Model
	VisionAPIConfig   *models.APIConfig

	EmbeddingModelEntity *entity.TenantModel
	EmbeddingModelInfo   *models.Model
	EmbeddingAPIConfig   *models.APIConfig

	RerankModelEntity *entity.TenantModel
	RerankModelInfo   *models.Model
	RerankAPIConfig   *models.APIConfig

	OCRModelEntity *entity.TenantModel
	OCRModelInfo   *models.Model
	OCRAPIConfig   *models.APIConfig

	FileParseModelEntity *entity.TenantModel
	FileParseModelInfo   *models.Model
	FileParseAPIConfig   *models.APIConfig

	TTSParseModelEntity *entity.TenantModel
	TTSParseModelInfo   *models.Model
	TTSParseAPIConfig   *models.APIConfig

	ASRParseModelEntity *entity.TenantModel
	ASRParseModelInfo   *models.Model
	ASRParseAPIConfig   *models.APIConfig
}
