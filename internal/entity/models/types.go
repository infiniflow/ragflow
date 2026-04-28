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
	Encode(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error)
	// Rerank calculates similarity scores between query and texts
	Rerank(modelName *string, query string, texts []string, apiConfig *APIConfig) ([]float64, error)
	// ListModels List supported models
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
	Status      string `json:"status"`
}

type ChatConfig struct {
	Stream      *bool
	Thinking    *bool
	MaxTokens   *int
	Temperature *float64
	TopP        *float64
	DoSample    *bool
	Stop        *[]string
	ModelClass  *string
	Effort      *string
	Verbosity   *string
}

type APIConfig struct {
	ApiKey *string
	Region *string
}

type EmbeddingConfig struct {
}

// EmbeddingModel wraps a ModelDriver with embedding-specific configuration
type EmbeddingModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
}

// NewEmbeddingModel creates a new EmbeddingModel
func NewEmbeddingModel(driver ModelDriver, modelName *string, apiConfig *APIConfig) *EmbeddingModel {
	return &EmbeddingModel{
		ModelDriver: driver,
		ModelName:   modelName,
		APIConfig:   apiConfig,
	}
}

// RerankModel wraps a ModelDriver with rerank-specific configuration
type RerankModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
}

// NewRerankModel creates a new RerankModel
func NewRerankModel(driver ModelDriver, modelName *string, apiConfig *APIConfig) *RerankModel {
	return &RerankModel{
		ModelDriver: driver,
		ModelName:   modelName,
		APIConfig:   apiConfig,
	}
}

// Rerank calculates similarity between query and texts
func (r *RerankModel) Rerank(query string, texts []string, apiConfig *APIConfig) ([]float64, error) {
	return r.ModelDriver.Rerank(r.ModelName, query, texts, apiConfig)
}

// ChatModel wraps a ModelDriver with chat-specific configuration
type ChatModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
}

// NewChatModel creates a new ChatModel
func NewChatModel(driver ModelDriver, modelName *string, apiConfig *APIConfig) *ChatModel {
	return &ChatModel{
		ModelDriver: driver,
		ModelName:   modelName,
		APIConfig:   apiConfig,
	}
}
