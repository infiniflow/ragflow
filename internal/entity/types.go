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

package entity

// ModelType represents the type of model
type ModelType string

const (
	// ModelTypeChat chat model
	ModelTypeChat ModelType = "chat"
	// ModelTypeEmbedding embedding model
	ModelTypeEmbedding ModelType = "embedding"
	// ModelTypeSpeech2Text speech to text model
	ModelTypeSpeech2Text ModelType = "speech2text"
	// ModelTypeImage2Text image to text model
	ModelTypeImage2Text ModelType = "image2text"
	// ModelTypeRerank rerank model
	ModelTypeRerank ModelType = "rerank"
	// ModelTypeTTS text to speech model
	ModelTypeTTS ModelType = "tts"
	// ModelTypeOCR optical character recognition model
	ModelTypeOCR ModelType = "ocr"
)

// ModelCredentials holds the credentials for a model
type ModelCredentials struct {
	ProviderName string
	ModelName    string
	APIKey       string
}
