package models

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

const defaultMaxRerankTokens = 8196

// ErrRerankTokenLimitPolicy identifies invalid token-limit configuration or oversized rerank input.
var ErrRerankTokenLimitPolicy = errors.New("rerank token limit policy")

// Message represents a chat message with role and content
//
//	is interface{} to support different formats:
//	 - string: plain text message (e.g., "Hello")
//	 - []interface{}: multimodal content array where each element is map[string]interface{}
//	   (e.g., [{"type": "text", "text": "..."}, {"type": "image_url", "image_url": {"url": "..."}}])
type Message struct {
	Role         string                   `json:"role"`
	Content      interface{}              `json:"content"`
	Name         interface{}              `json:"name,omitempty"`
	ToolCallID   string                   `json:"tool_call_id,omitempty"`
	ToolCalls    []map[string]interface{} `json:"tool_calls,omitempty"`
	FunctionCall interface{}              `json:"function_call,omitempty"`
	Refusal      interface{}              `json:"refusal,omitempty"`
	Audio        interface{}              `json:"audio,omitempty"`
}

// ToolCallSession mirrors Python's common.mcp_tool_call_conn.ToolCallSession protocol.
type ToolCallSession interface {
	ToolCall(name string, arguments map[string]interface{}) (string, error)
}

// ModelDriver interface for model functionality
type ModelDriver interface {
	NewInstance(baseURL map[string]string) ModelDriver

	Name() string

	// ChatWithMessages sends multiple messages synchronously
	ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error)
	// ChatStreamlyWithSender sends multiple messages asynchronously
	ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error
	// Embed a list of texts into embeddings
	Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error)
	// Rerank calculates similarity scores between query and texts
	Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error)
	// TranscribeAudio transcribe audio
	TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error)
	TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error
	// AudioSpeech convert text to audio
	AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error)
	AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error
	// OCRFile OCR file
	OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error)
	// ParseFile parse file
	ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error)
	// ListModels List supported models
	ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error)

	Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error)

	CheckConnection(ctx context.Context, apiConfig *APIConfig) error

	ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error)

	ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error)
}

type ChatResponse struct {
	Answer        *string                  `json:"answer"`
	ReasonContent *string                  `json:"reason_content"`
	ToolCalls     []map[string]interface{} `json:"tool_calls,omitempty"`
	Usage         *TokenUsage              `json:"usage,omitempty"`
}

// TokenUsage holds token usage split for one LLM call. Consumed by
// LLMBundle for accurate Langfuse reporting and run aggregation.
// Mirrors Python's common.token_utils.usage_from_response() split.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"  mapstructure:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"  mapstructure:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"  mapstructure:"total_tokens"`
}

type EmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
	// TokenCount is what this input cost, taken from the provider's reported
	// usage. Embedding APIs report usage per *request*, not per input, so the
	// ingest path distributes the request total across the inputs it sent
	// (internal/ingestion/task/embedder.go). It stays 0 for providers that
	// report no usage at all — by design, rather than inventing a number.
	TokenCount int `json:"token_count"`
}

type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type RerankResponse struct {
	Data []RerankResult `json:"data"`
}

type ASRResponse struct {
	Text string `json:"text"`
}

type TTSResponse struct {
	Audio []byte `json:"audio"`
	// MediaType is the MIME type of Audio (e.g. "audio/mpeg", "audio/wav").
	// Empty means the caller's default (audio/mpeg).
	MediaType string `json:"media_type,omitempty"`
}

type OCRFileResponse struct {
	Text *string `json:"text"`
}

type ListModelResponse struct {
	Name          string         `json:"name"`
	ContextLength *int           `json:"context_length"`
	MaxOutput     *int           `json:"max_output"`
	ModelTypes    []string       `json:"model_types"`
	Thinking      *ModelThinking `json:"thinking"`
	MaxDimension  *int           `json:"max_dimension"`  // used by embedding models
	MaxBatchSize  *int           `json:"max_batch_size"` // used by embedding models
	Dimensions    []int          `json:"dimensions"`
}

type ParseFileResponse struct {
	TaskID string `json:"task_id"`
}

type ListTaskStatus struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type TaskSegment struct {
	Index   int    `json:"index"`
	Content string `json:"content"`
}

type TaskResponse struct {
	Segments []TaskSegment `json:"segments"`
}

type ModelListItem struct {
	ID            string `json:"id"`
	ContextLength *int   `json:"context_length"`
	Object        string `json:"object"`
	OwnedBy       string `json:"owned_by"`
}

type ModelList struct {
	Object string          `json:"object"`
	Models []ModelListItem `json:"data"`
}

// URLSuffix represents the URL suffixes for different API endpoints
type URLSuffix struct {
	Chat          string `json:"chat"`
	AsyncChat     string `json:"async_chat"`
	AsyncResult   string `json:"async_result"`
	Embedding     string `json:"embedding"`
	Rerank        string `json:"rerank"`
	TTS           string `json:"tts"`
	ASR           string `json:"asr"`
	OCR           string `json:"ocr"`
	DocumentParse string `json:"doc_parse"`
	Models        string `json:"models"`
	Balance       string `json:"balance"`
	Files         string `json:"files"`
	Status        string `json:"status"`
	Tasks         string `json:"tasks"`
	Task          string `json:"task"`
}

type ChatConfig struct {
	Stream          *bool
	Vision          *bool
	Thinking        *bool
	MaxTokens       *int
	Temperature     *float64
	TopP            *float64
	DoSample        *bool
	Stop            *[]string
	ModelClass      *string
	Effort          *string
	Verbosity       *string
	Tools           interface{}               `json:"tools,omitempty"`
	ToolChoice      *string                   `json:"tool_choice,omitempty"`
	ToolChoiceValue any                       `json:"-"`
	ToolCallsResult *[]map[string]interface{} `json:"-"`
	// UsageResult receives the token usage extracted from the final
	// streaming chunk when stream_options.include_usage is true.
	// The ChatStreamlyWithSender driver writes to this pointer (if
	// non-nil) after the stream completes; callers read it the same
	// way they read ToolCallsResult.
	UsageResult *TokenUsage `json:"-"`
}

type APIConfig struct {
	ApiKey  *string
	Region  *string
	BaseURL *string
}

type EmbedRequest struct {
	Texts  []string // for text
	Images [][]byte // for image
	Urls   []string // for image
	// Query selects the query-side encoding for providers that embed queries
	// and documents differently (Python's LLMBundle.encode_queries vs encode):
	// Cohere/Bedrock-Cohere input_type=search_query, Voyage input_type=query,
	// Jina task=retrieval.query, NVIDIA input_type=query, DashScope
	// text_type=query. Providers without an asymmetric mode ignore it.
	Query bool
}

type EmbeddingConfig struct {
	Dimension      int
	EncodingFormat string
}

type RerankRequest struct {
	Query         string  // for text question
	ImageQuery    []byte  // for image
	ImageQueryURL *string // for image

	Documents []string // for text candidates
	Images    [][]byte // for image candidates
	ImageURLs []string // for image candidates
}

type RerankConfig struct {
	TopN int
}

type ASRConfig struct {
	Params map[string]interface{} `json:"params"`
}

type TTSConfig struct {
	Format string                 `json:"format"`
	Params map[string]interface{} `json:"params"`
}

type OCRConfig struct {
	Algorithm string
}

type ParseFileConfig struct {
	ParseMethod string `json:"parse_method"`
	Backend     string `json:"backend"`
	ServerURL   string `json:"server_url"`
}

// EmbeddingModel wraps a ModelDriver with embedding-specific configuration
type EmbeddingModel struct {
	ModelDriver  ModelDriver
	ModelName    *string
	APIConfig    *APIConfig
	MaxTokens    int  // Max input tokens for the embedding model, used for text truncation
	MaxBatchSize *int // Max texts per Embed request; nil means "resolve from provider capability at use site"
	info         *ModelInfo
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

// NewEmbeddingModelWithInfo creates an embedding model with resolved metadata.
func NewEmbeddingModelWithInfo(driver ModelDriver, modelName *string, apiConfig *APIConfig, maxTokens int, info *ModelInfo) *EmbeddingModel {
	model := NewEmbeddingModel(driver, modelName, apiConfig, maxTokens)
	model.info = cloneModelInfo(info)
	if info != nil && info.Catalog != nil {
		model.MaxBatchSize = info.Catalog.MaxBatchSize
	}
	return model
}

// Info returns metadata for this selected embedding model.
func (m *EmbeddingModel) Info() *ModelInfo {
	if m == nil {
		return nil
	}
	return cloneModelInfo(m.info)
}

// ResolveBatchSize returns the safe maximum texts per Embed request. The
// effective limit cannot exceed either the model's declared maximum or the
// provider/runtime batch capability; these values can differ (for example, a
// provider model config can declare max_batch_size=32 while its driver only
// accepts the default batch size of 16).
func (m *EmbeddingModel) ResolveBatchSize() int {
	var name string
	if m != nil && m.ModelName != nil {
		name = *m.ModelName
	}
	batchSize := GetEmbeddingBatchSize(name)
	if m != nil && m.MaxBatchSize != nil && *m.MaxBatchSize > 0 && *m.MaxBatchSize < batchSize {
		return *m.MaxBatchSize
	}
	return batchSize
}

// ResolveMaxTokens is ResolveBatchSize's counterpart for the input window: the
// model's own declaration wins, then the provider catalog's context_length, then
// 0, which tells the caller to apply its own default. Deliberately not 8192:
// the catalog has embedding models with 512-token windows, and overshooting a
// window is a rejected request while undershooting only truncates.
func (m *EmbeddingModel) ResolveMaxTokens() int {
	if m == nil {
		return 0
	}
	if m.MaxTokens > 0 {
		return m.MaxTokens
	}
	var name string
	if m.ModelName != nil {
		name = *m.ModelName
	}
	return GetEmbeddingMaxTokens(name)
}

// ResolveTokenizerID returns the tokenizer family declared for this model, or ""
// when the model's own tokenizer is unknown (the caller then counts with cl100k
// and a calibrated ratio).
func (m *EmbeddingModel) ResolveTokenizerID() string {
	if m == nil {
		return ""
	}
	var name string
	if m.ModelName != nil {
		name = *m.ModelName
	}
	return GetModelTokenizer(name)
}

// QuotaKey names the deployment this embedding model counts against: endpoint,
// region, model name and an API-key prefix. The tokenizer belongs to the model, but
// what a provider accepts is per deployment - the same model behind two endpoints
// can have different windows - so everything that learns a real/own token ratio
// (the ingest embedder, the dataset-nav embedder, the knowledge-compiler embedder)
// has to key that ratio the same way; otherwise each path re-learns the same
// rejection and none of them tightens for the others.
func (m *EmbeddingModel) QuotaKey() string {
	if m == nil {
		return ""
	}
	var baseURL, region, apiKey, modelName string
	if cfg := m.APIConfig; cfg != nil {
		if cfg.BaseURL != nil {
			baseURL = *cfg.BaseURL
		}
		if cfg.Region != nil {
			region = *cfg.Region
		}
		if cfg.ApiKey != nil {
			apiKey = *cfg.ApiKey
		}
	}
	if m.ModelName != nil {
		modelName = *m.ModelName
	}
	sum := sha256.Sum256([]byte(apiKey))
	return fmt.Sprintf("%s|%s|%s|%x", baseURL, region, modelName, sum[:8])
}

// RerankModel wraps a ModelDriver with rerank-specific configuration
type RerankModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
	MaxTokens   int

	limiter    tokenizer.Limiter
	limiterErr error
	info       *ModelInfo
}

// NewRerankModel creates a new RerankModel
func NewRerankModel(driver ModelDriver, modelName *string, apiConfig *APIConfig, maxTokens int) *RerankModel {
	if maxTokens <= 0 {
		maxTokens = defaultMaxRerankTokens
	}
	tokenizerID := ""
	if modelName != nil {
		tokenizerID = GetModelTokenizer(*modelName)
	}
	return &RerankModel{
		ModelDriver: driver,
		ModelName:   modelName,
		APIConfig:   apiConfig,
		MaxTokens:   maxTokens,
		limiter:     tokenizer.LimiterFor(tokenizerID, "", tokenizer.DefaultCalibration()),
		limiterErr:  tokenizer.RefuseUnavailableCounter(tokenizerID, "rerank"),
	}
}

// NewRerankModelWithInfo creates a rerank model with resolved metadata.
func NewRerankModelWithInfo(driver ModelDriver, modelName *string, apiConfig *APIConfig, maxTokens int, info *ModelInfo) *RerankModel {
	model := NewRerankModel(driver, modelName, apiConfig, maxTokens)
	model.info = cloneModelInfo(info)
	return model
}

// Info returns metadata for this selected rerank model.
func (r *RerankModel) Info() *ModelInfo {
	if r == nil {
		return nil
	}
	return cloneModelInfo(r.info)
}

// Rerank calculates similarity between query and texts. Rerank input is measured
// and truncated with the model's declared tokenizer, just like embedding input.
func (r *RerankModel) Rerank(ctx context.Context, request RerankRequest, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if r == nil || r.ModelDriver == nil {
		return nil, errors.New("rerank model: driver is nil")
	}
	maxTokens := r.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxRerankTokens
	}
	if r.limiterErr != nil {
		return nil, r.limiterErr
	}
	limiter := r.limiter
	counter := limiter.Counter()
	effectiveMaxTokens := limiter.Limit(maxTokens)
	mode := strings.ToLower(strings.TrimSpace(common.GetEnv(common.EnvRerankTokenLimitMode)))
	if mode == "" {
		mode = "truncate"
	}
	if mode != "truncate" && mode != "passthrough" && mode != "raise_error" {
		return nil, fmt.Errorf("%w: invalid %s %q; expected %q, %q, or %q", ErrRerankTokenLimitPolicy, common.EnvRerankTokenLimitMode, mode, "truncate", "passthrough", "raise_error")
	}
	if mode != "passthrough" && len(request.Documents) > 0 {
		queryTokens := counter.Count(request.Query)
		if mode == "truncate" {
			documentBudget := max(effectiveMaxTokens-queryTokens, 0)
			documentTokens := tokenizer.OverLimitLadder(documentBudget)[0]
			documents := make([]string, len(request.Documents))
			for i, document := range request.Documents {
				documents[i] = counter.TrimToLimit(document, documentTokens)
			}
			request.Documents = documents
		} else {
			for i, document := range request.Documents {
				inputTokens := queryTokens + counter.Count(document)
				if inputTokens > effectiveMaxTokens {
					return nil, fmt.Errorf("%w: rerank input at document index %d has %d tokens, exceeding the configured maximum of %d", ErrRerankTokenLimitPolicy, i, inputTokens, effectiveMaxTokens)
				}
			}
		}
	}
	return r.ModelDriver.Rerank(ctx, r.ModelName, request, r.APIConfig, rerankConfig, modelUsage)
}

// OCRModel wraps a ModelDriver with optical-character-recognition configuration.
type OCRModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
	info        *ModelInfo
}

// NewOCRModel creates an OCR model wrapper.
func NewOCRModel(driver ModelDriver, modelName *string, apiConfig *APIConfig) *OCRModel {
	return &OCRModel{ModelDriver: driver, ModelName: modelName, APIConfig: apiConfig}
}

// NewOCRModelWithInfo creates an OCR model with resolved metadata.
func NewOCRModelWithInfo(driver ModelDriver, modelName *string, apiConfig *APIConfig, info *ModelInfo) *OCRModel {
	model := NewOCRModel(driver, modelName, apiConfig)
	model.info = cloneModelInfo(info)
	return model
}

// Info returns metadata for this selected OCR model.
func (m *OCRModel) Info() *ModelInfo {
	if m == nil {
		return nil
	}
	return cloneModelInfo(m.info)
}

// OCRFile extracts text from an image or document.
func (m *OCRModel) OCRFile(ctx context.Context, content []byte, url *string, config *OCRConfig, usage *common.ModelUsage) (*OCRFileResponse, error) {
	if m == nil || m.ModelDriver == nil {
		return nil, errors.New("OCR model: driver is nil")
	}
	return m.ModelDriver.OCRFile(ctx, m.ModelName, content, url, m.APIConfig, config, usage)
}

// ASRModel wraps a ModelDriver with speech-to-text configuration.
type ASRModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
	info        *ModelInfo
}

// NewASRModel creates a new ASRModel.
func NewASRModel(driver ModelDriver, modelName *string, apiConfig *APIConfig) *ASRModel {
	return &ASRModel{ModelDriver: driver, ModelName: modelName, APIConfig: apiConfig}
}

// NewASRModelWithInfo creates an ASR model with resolved metadata.
func NewASRModelWithInfo(driver ModelDriver, modelName *string, apiConfig *APIConfig, info *ModelInfo) *ASRModel {
	model := NewASRModel(driver, modelName, apiConfig)
	model.info = cloneModelInfo(info)
	return model
}

// Info returns metadata for this selected ASR model.
func (m *ASRModel) Info() *ModelInfo {
	if m == nil {
		return nil
	}
	return cloneModelInfo(m.info)
}

// Transcribe converts audio to text.
func (m *ASRModel) Transcribe(ctx context.Context, audioFile *string, config *ASRConfig, usage *common.ModelUsage) (*ASRResponse, error) {
	return m.ModelDriver.TranscribeAudio(ctx, m.ModelName, audioFile, m.APIConfig, config, usage)
}

// TranscribeWithSender streams transcription results through sender.
func (m *ASRModel) TranscribeWithSender(ctx context.Context, audioFile *string, config *ASRConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return m.ModelDriver.TranscribeAudioWithSender(ctx, m.ModelName, audioFile, m.APIConfig, config, usage, sender)
}

// TTSModel wraps a ModelDriver with text-to-speech configuration.
type TTSModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
	info        *ModelInfo
}

// NewTTSModel creates a new TTSModel.
func NewTTSModel(driver ModelDriver, modelName *string, apiConfig *APIConfig) *TTSModel {
	return &TTSModel{ModelDriver: driver, ModelName: modelName, APIConfig: apiConfig}
}

// NewTTSModelWithInfo creates a TTS model with resolved metadata.
func NewTTSModelWithInfo(driver ModelDriver, modelName *string, apiConfig *APIConfig, info *ModelInfo) *TTSModel {
	model := NewTTSModel(driver, modelName, apiConfig)
	model.info = cloneModelInfo(info)
	return model
}

// Info returns metadata for this selected TTS model.
func (m *TTSModel) Info() *ModelInfo {
	if m == nil {
		return nil
	}
	return cloneModelInfo(m.info)
}

// Speech converts text to audio.
func (m *TTSModel) Speech(ctx context.Context, audioContent *string, config *TTSConfig, usage *common.ModelUsage) (*TTSResponse, error) {
	return m.ModelDriver.AudioSpeech(ctx, m.ModelName, audioContent, m.APIConfig, config, usage)
}

// SpeechWithSender streams synthesized audio through sender.
func (m *TTSModel) SpeechWithSender(ctx context.Context, audioContent *string, config *TTSConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	return m.ModelDriver.AudioSpeechWithSender(ctx, m.ModelName, audioContent, m.APIConfig, config, usage, sender)
}

// ToolConfig bundles tool-calling configuration for a ChatModel.
type ToolConfig struct {
	Tools           string          // JSON-encoded tools list
	MaxRounds       int             // max tool-calling rounds (default: 5)
	MaxRetries      int             // max retries on failure (default: 3)
	ToolCallSession ToolCallSession // session that executes tool calls
	// TerminalTools names tools whose successful result is already the final
	// answer. When a round executes one of them, the loop stops and returns
	// that result instead of feeding it back for another model round. Mirrors
	// Python's chat_mdl.terminal_tools short-circuit (chat_model.py:619-627).
	// Empty disables the short-circuit (existing behaviour).
	TerminalTools map[string]struct{}
}

// ChatModel wraps a ModelDriver with chat-specific configuration
type ChatModel struct {
	ModelDriver ModelDriver
	ModelName   *string
	APIConfig   *APIConfig
	ToolConfig  *ToolConfig
	info        *ModelInfo
}

// NewChatModel creates a new ChatModel
func NewChatModel(driver ModelDriver, modelName *string, apiConfig *APIConfig) *ChatModel {
	return &ChatModel{
		ModelDriver: driver,
		ModelName:   modelName,
		APIConfig:   apiConfig,
	}
}

// NewChatModelWithInfo creates a chat model with resolved metadata.
func NewChatModelWithInfo(driver ModelDriver, modelName *string, apiConfig *APIConfig, info *ModelInfo) *ChatModel {
	model := NewChatModel(driver, modelName, apiConfig)
	model.info = cloneModelInfo(info)
	return model
}

// Info returns metadata for this selected chat model.
func (m *ChatModel) Info() *ModelInfo {
	if m == nil {
		return nil
	}
	return cloneModelInfo(m.info)
}

// ChatWithMessages sends a non-streaming chat request through the model wrapper.
func (m *ChatModel) ChatWithMessages(ctx context.Context, messages []Message, config *ChatConfig, usage *common.ModelUsage) (*ChatResponse, error) {
	if m == nil || m.ModelDriver == nil {
		return nil, errors.New("chat model: driver is nil")
	}
	config = m.withDefaults(config)
	if err := m.validateMaxOutput(config); err != nil {
		return nil, err
	}
	modelName := ""
	if m.ModelName != nil {
		modelName = *m.ModelName
	}
	return m.ModelDriver.ChatWithMessages(ctx, modelName, messages, m.APIConfig, config, usage)
}

// ChatStreamlyWithSender streams chat deltas through sender.
func (m *ChatModel) ChatStreamlyWithSender(ctx context.Context, messages []Message, config *ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	if m == nil || m.ModelDriver == nil {
		return errors.New("chat model: driver is nil")
	}
	config = m.withDefaults(config)
	if err := m.validateMaxOutput(config); err != nil {
		return err
	}
	modelName := ""
	if m.ModelName != nil {
		modelName = *m.ModelName
	}
	return m.ModelDriver.ChatStreamlyWithSender(ctx, modelName, messages, m.APIConfig, config, usage, sender)
}

func (m *ChatModel) withDefaults(config *ChatConfig) *ChatConfig {
	if config == nil {
		config = &ChatConfig{}
	}
	if m.info == nil {
		return config
	}
	if config.ModelClass == nil && m.info.ModelClass != "" {
		modelClass := m.info.ModelClass
		config.ModelClass = &modelClass
	}
	if config.Thinking == nil && m.info.Thinking != nil {
		thinking := m.info.Thinking.DefaultValue
		config.Thinking = &thinking
	}
	return config
}

func (m *ChatModel) validateMaxOutput(config *ChatConfig) error {
	if m == nil || m.info == nil || m.info.MaxOutput <= 0 || config == nil || config.MaxTokens == nil {
		return nil
	}
	if *config.MaxTokens > m.info.MaxOutput {
		return fmt.Errorf("chat max_tokens %d exceeds model max_output %d", *config.MaxTokens, m.info.MaxOutput)
	}
	return nil
}

// BindTools registers tools for the ChatModel to call.
// Mirrors Python's Base.bind_tools() in rag/llm/chat_model.py.
func (cm *ChatModel) BindTools(session ToolCallSession, tools interface{}) {
	// Serialize tools to JSON if it's a list/map.
	toolsJSON := ""
	switch v := tools.(type) {
	case string:
		toolsJSON = v
	case []byte:
		toolsJSON = string(v)
	default:
		if b, err := json.Marshal(tools); err == nil {
			toolsJSON = string(b)
		}
	}
	cm.ToolConfig = &ToolConfig{
		Tools:           toolsJSON,
		MaxRounds:       defaultMaxRounds,
		MaxRetries:      defaultMaxRetries,
		ToolCallSession: session,
	}
}

// SetTerminalTools marks the named tools as terminal: once one executes
// successfully, the tool loop stops and returns its result as the final answer
// rather than re-invoking the model. Mirrors Python
// `chat_mdl.mdl.terminal_tools = {...}`. Call after BindTools.
func (cm *ChatModel) SetTerminalTools(names ...string) {
	if cm.ToolConfig == nil {
		return
	}
	term := make(map[string]struct{}, len(names))
	for _, n := range names {
		term[n] = struct{}{}
	}
	cm.ToolConfig.TerminalTools = term
}

// ModelInfo describes the selected model without exposing its driver
// credentials. ContextLength is the input/context window; MaxOutput is the
// provider's maximum generated-token budget.
type ModelInfo struct {
	ID            string
	Name          string
	ProviderName  string
	InstanceID    string
	InstanceName  string
	ModelTypes    []string
	ContextLength int
	MaxOutput     int
	SupportsTools bool
	ModelClass    string
	Thinking      *ModelThinking
	Catalog       *Model
}

func cloneModelInfo(info *ModelInfo) *ModelInfo {
	if info == nil {
		return nil
	}
	copy := *info
	copy.ModelTypes = append([]string(nil), info.ModelTypes...)
	if info.Thinking != nil {
		thinking := *info.Thinking
		copy.Thinking = &thinking
	}
	if info.Catalog != nil {
		catalog := *info.Catalog
		catalog.ModelTypes = append([]string(nil), info.Catalog.ModelTypes...)
		catalog.Dimensions = append([]int(nil), info.Catalog.Dimensions...)
		catalog.Alias = append([]string(nil), info.Catalog.Alias...)
		copy.Catalog = &catalog
	}
	return &copy
}
