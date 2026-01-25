package model

// LLMFactories LLM factory model
type LLMFactories struct {
	Name   string  `gorm:"column:name;primaryKey;size:128" json:"name"`
	Logo   *string `gorm:"column:logo;type:longtext" json:"logo,omitempty"`
	Tags   string  `gorm:"column:tags;size:255;not null;index" json:"tags"`
	Rank   int64   `gorm:"column:rank;default:0" json:"rank"`
	Status *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (LLMFactories) TableName() string {
	return "llm_factories"
}

// LLM LLM model
type LLM struct {
	LLMName   string  `gorm:"column:llm_name;size:128;not null;primaryKey" json:"llm_name"`
	ModelType string  `gorm:"column:model_type;size:128;not null;index" json:"model_type"`
	FID       string  `gorm:"column:fid;size:128;not null;primaryKey" json:"fid"`
	MaxTokens int64   `gorm:"column:max_tokens;default:0" json:"max_tokens"`
	Tags      string  `gorm:"column:tags;size:255;not null;index" json:"tags"`
	IsTools   bool    `gorm:"column:is_tools;default:false" json:"is_tools"`
	Status    *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (LLM) TableName() string {
	return "llm"
}

// TenantLLM tenant LLM model
type TenantLLM struct {
	TenantID    string `gorm:"column:tenant_id;size:32;not null;primaryKey" json:"tenant_id"`
	LLMFactory  string `gorm:"column:llm_factory;size:128;not null;primaryKey" json:"llm_factory"`
	ModelType   string `gorm:"column:model_type;size:128;not null;index" json:"model_type"`
	LLMName     string `gorm:"column:llm_name;size:128;not null;primaryKey;default:" json:"llm_name"`
	APIKey      string `gorm:"column:api_key;type:longtext" json:"api_key,omitempty"`
	APIBase     string `gorm:"column:api_base;size:255" json:"api_base,omitempty"`
	MaxTokens   int64  `gorm:"column:max_tokens;default:8192;index" json:"max_tokens"`
	UsedTokens  int64  `gorm:"column:used_tokens;default:0;index" json:"used_tokens"`
	Status      string `gorm:"column:status;size:1;not null;default:1;index" json:"status"`
	BaseModel
}

// TableName specify table name
func (TenantLLM) TableName() string {
	return "tenant_llm"
}

// TenantLangfuse tenant langfuse model
type TenantLangfuse struct {
	TenantID  string `gorm:"column:tenant_id;primaryKey;size:32" json:"tenant_id"`
	SecretKey string `gorm:"column:secret_key;size:2048;not null;index" json:"secret_key"`
	PublicKey string `gorm:"column:public_key;size:2048;not null;index" json:"public_key"`
	Host      string `gorm:"column:host;size:128;not null;index" json:"host"`
	BaseModel
}

// TableName specify table name
func (TenantLangfuse) TableName() string {
	return "tenant_langfuse"
}