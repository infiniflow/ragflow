package dao

import (
	"ragflow/internal/model"
)

// TenantDAO tenant data access object
type TenantDAO struct{}

// NewTenantDAO create tenant DAO
func NewTenantDAO() *TenantDAO {
	return &TenantDAO{}
}

// GetJoinedTenantsByUserID get joined tenants by user ID
func (dao *TenantDAO) GetJoinedTenantsByUserID(userID string) ([]*TenantWithRole, error) {
	var results []*TenantWithRole

	err := DB.Model(&model.Tenant{}).
		Select("tenant.id as tenant_id, tenant.name, tenant.llm_id, tenant.embd_id, tenant.asr_id, tenant.img2txt_id, user_tenant.role").
		Joins("INNER JOIN user_tenant ON user_tenant.tenant_id = tenant.id").
		Where("user_tenant.user_id = ? AND user_tenant.status = ? AND user_tenant.role = ? AND tenant.status = ?", userID, "1", "normal", "1").
		Scan(&results).Error

	return results, err
}

// TenantWithRole tenant with role information
type TenantWithRole struct {
	TenantID string `gorm:"column:tenant_id" json:"tenant_id"`
	Name      string `gorm:"column:name" json:"name"`
	LLMID     string `gorm:"column:llm_id" json:"llm_id"`
	EmbDID    string `gorm:"column:embd_id" json:"embd_id"`
	ASRID      string `gorm:"column:asr_id" json:"asr_id"`
	Img2TxtID string `gorm:"column:img2txt_id" json:"img2txt_id"`
	Role      string `gorm:"column:role" json:"role"`
}

// TenantInfo tenant information with role (for owner tenant)
type TenantInfo struct {
	TenantID  string  `gorm:"column:tenant_id" json:"tenant_id"`
	Name      *string `gorm:"column:name" json:"name,omitempty"`
	LLMID     string  `gorm:"column:llm_id" json:"llm_id"`
	EmbDID    string  `gorm:"column:embd_id" json:"embd_id"`
	RerankID  string  `gorm:"column:rerank_id" json:"rerank_id"`
	ASRID     string  `gorm:"column:asr_id" json:"asr_id"`
	Img2TxtID string  `gorm:"column:img2txt_id" json:"img2txt_id"`
	TTSID     *string `gorm:"column:tts_id" json:"tts_id,omitempty"`
	ParserIDs string  `gorm:"column:parser_ids" json:"parser_ids"`
	Role      string  `gorm:"column:role" json:"role"`
}

// GetInfoByUserID get tenant information for the owner tenant of a user
func (dao *TenantDAO) GetInfoByUserID(userID string) ([]*TenantInfo, error) {
	var results []*TenantInfo

	err := DB.Model(&model.Tenant{}).
		Select("tenant.id as tenant_id, tenant.name, tenant.llm_id, tenant.embd_id, tenant.rerank_id, tenant.asr_id, tenant.img2txt_id, tenant.tts_id, tenant.parser_ids, user_tenant.role").
		Joins("INNER JOIN user_tenant ON user_tenant.tenant_id = tenant.id").
		Where("user_tenant.user_id = ? AND user_tenant.status = ? AND user_tenant.role = ? AND tenant.status = ?", userID, "1", "owner", "1").
		Scan(&results).Error

	return results, err
}

// GetByID gets tenant by ID
func (dao *TenantDAO) GetByID(id string) (*model.Tenant, error) {
	var tenant model.Tenant
	err := DB.Where("id = ? AND status = ?", id, "1").First(&tenant).Error
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}
