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
