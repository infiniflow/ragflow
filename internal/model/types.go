package model

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

// EmbeddingModel interface for embedding models
type EmbeddingModel interface {
	// Encode encodes a list of texts into embeddings
	Encode(texts []string) ([][]float64, error)
	// EncodeQuery encodes a single query string into embedding
	EncodeQuery(query string) ([]float64, error)
}

// ChatModel interface for chat models
type ChatModel interface {
	// Chat sends a message and returns response
	Chat(system string, history []map[string]string, genConf map[string]interface{}) (string, error)
	// ChatStreamly sends a message and streams response
	ChatStreamly(system string, history []map[string]string, genConf map[string]interface{}) (<-chan string, error)
}

// RerankModel interface for rerank models
type RerankModel interface {
	// Similarity calculates similarity between query and texts
	Similarity(query string, texts []string) ([]float64, error)
}

// ModelConfig represents configuration for a model
type ModelConfig struct {
	TenantID    string    `json:"tenant_id"`
	LLMFactory  string    `json:"llm_factory"`
	ModelType   ModelType `json:"model_type"`
	LLMName     string    `json:"llm_name"`
	APIKey      string    `json:"api_key"`
	APIBase     string    `json:"api_base"`
	MaxTokens   int64     `json:"max_tokens"`
	IsTools     bool      `json:"is_tools"`
}