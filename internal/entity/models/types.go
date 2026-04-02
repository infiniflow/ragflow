package models

// EmbeddingModel interface for embedding models
type ModelDriver interface {
	// Chat sends a message and returns response
	Chat(modelName, apiKey, message *string, genConf map[string]interface{}) (string, error)
	// AsyncChat sends a message asynchronously and returns a task ID
	AsyncChat(modelName, apiKey, message *string, genConf map[string]interface{}) (string, error)
	// GetAsyncChatResult gets the result of an async chat task
	GetAsyncChatResult(taskID, apiKey *string) (string, error)
	// ChatStreamly sends a message and streams response
	ChatStreamly(modelName, apiKey, message *string, genConf map[string]interface{}) (<-chan string, error)
	// Encode encodes a list of texts into embeddings
	EncodeToEmbedding(modelName, apiKey *string, texts []string) ([][]float64, error)
}

// URLSuffix represents the URL suffixes for different API endpoints
type URLSuffix struct {
	Chat        string `json:"chat"`
	AsyncChat   string `json:"async_chat"`
	AsyncResult string `json:"async_result"`
	Embedding   string `json:"embedding"`
	Rerank      string `json:"rerank"`
}
