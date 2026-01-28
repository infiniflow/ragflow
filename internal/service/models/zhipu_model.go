package models

import (
	"net/http"
	"ragflow/internal/model"
)

func init() {
	RegisterEmbeddingModelFactory("ZHIPU-AI", func(apiKey, apiBase, modelName string, httpClient *http.Client) model.EmbeddingModel {
		return &openAIEmbeddingModel{
			apiKey:     apiKey,
			apiBase:    apiBase,
			model:      modelName,
			httpClient: httpClient,
		}
	})
}