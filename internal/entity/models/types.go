package models

// Message represents a chat message with role and content
//
// Content is interface{} to support different formats:
//   - string: plain text message (e.g., "Hello")
//   - []interface{}: multimodal content array where each element is map[string]interface{}
//     (e.g., [{"type": "text", "text": "..."}, {"type": "image_url", "image_url": {"url": "..."}}])
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// EmbeddingModel interface for embedding models
type ModelDriver interface {
	NewInstance(baseURL map[string]string) ModelDriver

	Name() string

	// ChatWithMessages sends multiple messages with role and content
	ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error)
	// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
	// messages accepts []Message which supports multimodal content (e.g., [{"type": "text", "text": "..."}, {"type": "image_url", "image_url": {"url": "..."}}])
	ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error
	// Encode encodes a list of texts into embeddings
	Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error)
	// Rerank calculates similarity scores between query and texts
	Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error)
	// TranscribeAudio transcribe audio
	TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error)
	TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error
	// AudioSpeech convert audio to text
	AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error)
	AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error
	// OCRFile OCR file
	OCRFile(modelName *string, fileContent *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error)
	// ListModels List supported models
	ListModels(apiConfig *APIConfig) ([]string, error)

	Balance(apiConfig *APIConfig) (map[string]interface{}, error)

	CheckConnection(apiConfig *APIConfig) error
}

type ChatResponse struct {
	Answer        *string `json:"answer"`
	ReasonContent *string `json:"reason_content"`
}

type EmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type RerankResponse struct {
	Data []RerankResult `json:"data"`
}

type ASRResponse struct {
}

type TTSResponse struct {
}

type OCRResponse struct {
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
	Vision      *bool
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
	Dimension int
}

type RerankConfig struct {
	TopN int
}

type ASRConfig struct {
}

type TTSConfig struct {
}

type OCRConfig struct {
}

// EmbeddingModel wraps a ModelDriver with embedding-specific configuration
type EmbeddingModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
	MaxTokens   int // Max input tokens for the embedding model, used for text truncation
}

// NewEmbeddingModel creates a new EmbeddingModel
func NewEmbeddingModel(driver ModelDriver, modelName *string, apiConfig *APIConfig, maxTokens int) *EmbeddingModel {
	return &EmbeddingModel{
		ModelDriver: driver,
		ModelName:   modelName,
		APIConfig:   apiConfig,
		MaxTokens:   maxTokens,
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
func (r *RerankModel) Rerank(query string, texts []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return r.ModelDriver.Rerank(r.ModelName, query, texts, apiConfig, rerankConfig)
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
