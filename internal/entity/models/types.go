package models

// EmbeddingModel interface for embedding models
type ModelDriver interface {
	// Chat sends a message and returns response
	Chat(modelName, apiKey, message *string, modelConfig *ChatConfig) (*ChatResponse, error)
	// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
	ChatStreamlyWithSender(modelName, apiKey, message *string, modelConfig *ChatConfig, sender func(*string, *string) error) error
	// Encode encodes a list of texts into embeddings
	EncodeToEmbedding(modelName, apiKey *string, texts []string, embeddingConfig *EmbeddingConfig) ([][]float64, error)
}

type ChatResponse struct {
	Answer        *string `json:"answer"`
	ReasonContent *string `json:"reason_content"`
}

// URLSuffix represents the URL suffixes for different API endpoints
type URLSuffix struct {
	Chat        string `json:"chat"`
	AsyncChat   string `json:"async_chat"`
	AsyncResult string `json:"async_result"`
	Embedding   string `json:"embedding"`
	Rerank      string `json:"rerank"`
}

type ChatConfig struct {
	Stream      *bool
	Thinking    *bool
	MaxTokens   *int
	Temperature *float64
	TopP        *float64
	DoSample    *bool
	Stop        *[]string
	Region      *string
}

type EmbeddingConfig struct {
	Region *string
}
