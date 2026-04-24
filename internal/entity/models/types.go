package models

// Message represents a chat message with role
type Message struct {
	Role    string
	Content string
}

// EmbeddingModel interface for embedding models
type ModelDriver interface {
	Name() string

	// Chat sends a message and returns response
	Chat(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig) (*ChatResponse, error)
	// ChatWithMessages sends multiple messages with roles (system, user, etc.) and returns response
	ChatWithMessages(modelName string, apiKey *string, messages []Message, modelConfig *ChatConfig) (string, error)
	// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
	ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error
	// Encode encodes a list of texts into embeddings
	EncodeToEmbedding(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error)
	// List suppported models
	ListModels(apiConfig *APIConfig) ([]string, error)

	Balance(apiConfig *APIConfig) (map[string]interface{}, error)

	CheckConnection(apiConfig *APIConfig) error
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
	Models      string `json:"models"`
	Balance     string `json:"balance"`
	Files       string `json:"files"`
}

type ChatConfig struct {
	Stream      *bool
	Thinking    *bool
	MaxTokens   *int
	Temperature *float64
	TopP        *float64
	DoSample    *bool
	Stop        *[]string
}

type APIConfig struct {
	ApiKey *string
	Region *string
}

type EmbeddingConfig struct {
}
