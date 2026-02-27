package model

// Chat chat model (mapped to dialog table)
type Chat struct {
	ID                    string          `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID              string          `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name                  *string         `gorm:"column:name;size:255;index" json:"name,omitempty"`
	Description           *string         `gorm:"column:description;type:longtext" json:"description,omitempty"`
	Icon                  *string         `gorm:"column:icon;type:longtext" json:"icon,omitempty"`
	Language              *string         `gorm:"column:language;size:32;index" json:"language,omitempty"`
	LLMID                 string          `gorm:"column:llm_id;size:128;not null" json:"llm_id"`
	LLMSetting            JSONMap         `gorm:"column:llm_setting;type:json;not null;default:'{\"temperature\":0.1,\"top_p\":0.3,\"frequency_penalty\":0.7,\"presence_penalty\":0.4,\"max_tokens\":512}'" json:"llm_setting"`
	PromptType            string          `gorm:"column:prompt_type;size:16;not null;default:simple;index" json:"prompt_type"`
	PromptConfig          JSONMap         `gorm:"column:prompt_config;type:json;not null;default:'{\"system\":\"\",\"prologue\":\"Hi! I'm your assistant. What can I do for you?\",\"parameters\":[],\"empty_response\":\"Sorry! No relevant content was found in the knowledge base!\"}'" json:"prompt_config"`
	MetaDataFilter        *JSONMap        `gorm:"column:meta_data_filter;type:json" json:"meta_data_filter,omitempty"`
	SimilarityThreshold   float64         `gorm:"column:similarity_threshold;default:0.2" json:"similarity_threshold"`
	VectorSimilarityWeight float64        `gorm:"column:vector_similarity_weight;default:0.3" json:"vector_similarity_weight"`
	TopN                  int64           `gorm:"column:top_n;default:6" json:"top_n"`
	TopK                  int64           `gorm:"column:top_k;default:1024" json:"top_k"`
	DoRefer               string          `gorm:"column:do_refer;size:1;not null;default:1" json:"do_refer"`
	RerankID              string          `gorm:"column:rerank_id;size:128;not null;default:''" json:"rerank_id"`
	KBIDs                 JSONSlice       `gorm:"column:kb_ids;type:json;not null;default:'[]'" json:"kb_ids"`
	Status                *string         `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (Chat) TableName() string {
	return "dialog"
}

// Conversation conversation model
type Conversation struct {
	ID        string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	DialogID  string   `gorm:"column:dialog_id;size:32;not null;index" json:"dialog_id"`
	Name      *string  `gorm:"column:name;size:255;index" json:"name,omitempty"`
	Message   JSONMap  `gorm:"column:message;type:json" json:"message,omitempty"`
	Reference JSONMap  `gorm:"column:reference;type:json;default:'[]'" json:"reference"`
	UserID    *string  `gorm:"column:user_id;size:255;index" json:"user_id,omitempty"`
	BaseModel
}

// TableName specify table name
func (Conversation) TableName() string {
	return "conversation"
}