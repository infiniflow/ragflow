package dao

import (
	"time"

	"ragflow/internal/model"
)

// UserTenantDAO user tenant data access object
type UserTenantDAO struct{}

// NewUserTenantDAO create user tenant DAO
func NewUserTenantDAO() *UserTenantDAO {
	return &UserTenantDAO{}
}

// Create create user tenant relationship
func (dao *UserTenantDAO) Create(userTenant *model.UserTenant) error {
	return DB.Create(userTenant).Error
}

// GetByID get user tenant relationship by ID
func (dao *UserTenantDAO) GetByID(id string) (*model.UserTenant, error) {
	var userTenant model.UserTenant
	err := DB.Where("id = ? AND status = ?", id, "1").First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// Update update user tenant relationship
func (dao *UserTenantDAO) Update(userTenant *model.UserTenant) error {
	return DB.Save(userTenant).Error
}

// Delete delete user tenant relationship (soft delete by setting status to "0")
func (dao *UserTenantDAO) Delete(id string) error {
	return DB.Model(&model.UserTenant{}).Where("id = ?", id).Update("status", "0").Error
}

// GetByTenantID get user tenant relationships by tenant ID with user details
// Similar to Python UserTenantService.get_by_tenant_id
func (dao *UserTenantDAO) GetByTenantID(tenantID string) ([]*UserTenantDetail, error) {
	var results []*UserTenantDetail

	err := DB.Model(&model.UserTenant{}).
		Select("user_tenant.id, user_tenant.user_id, user_tenant.status, user_tenant.role, user.nickname, user.email, user.avatar, user.is_authenticated, user.is_active, user.is_anonymous, user.status as user_status, user.update_date, user.is_superuser").
		Joins("INNER JOIN user ON user.id = user_tenant.user_id").
		Where("user_tenant.tenant_id = ? AND user_tenant.status = ? AND user_tenant.role != ?", tenantID, "1", "owner").
		Scan(&results).Error

	return results, err
}

// GetTenantsByUserID get tenants by user ID with tenant details
// Similar to Python UserTenantService.get_tenants_by_user_id
func (dao *UserTenantDAO) GetTenantsByUserID(userID string) ([]*TenantDetail, error) {
	var results []*TenantDetail

	err := DB.Model(&model.UserTenant{}).
		Select("user_tenant.tenant_id, user_tenant.role, user.nickname, user.email, user.avatar, user.update_date").
		Joins("INNER JOIN user ON user.id = user_tenant.tenant_id").
		Where("user_tenant.user_id = ? AND user_tenant.status = ?", userID, "1").
		Scan(&results).Error

	return results, err
}

// GetUserTenantRelationByUserID get user tenant relationships by user ID
// Similar to Python UserTenantService.get_user_tenant_relation_by_user_id
func (dao *UserTenantDAO) GetUserTenantRelationByUserID(userID string) ([]*model.UserTenant, error) {
	var relations []*model.UserTenant
	err := DB.Where("user_id = ? AND status = ?", userID, "1").Find(&relations).Error
	return relations, err
}

// GetNumMembers get number of members in a tenant (excluding owner)
// Similar to Python UserTenantService.get_num_members
func (dao *UserTenantDAO) GetNumMembers(tenantID string) (int64, error) {
	var count int64
	err := DB.Model(&model.UserTenant{}).
		Where("tenant_id = ? AND status = ? AND role != ?", tenantID, "1", "owner").
		Count(&count).Error
	return count, err
}

// FilterByTenantAndUserID filter user tenant relationship by tenant ID and user ID
// Similar to Python UserTenantService.filter_by_tenant_and_user_id
func (dao *UserTenantDAO) FilterByTenantAndUserID(tenantID, userID string) (*model.UserTenant, error) {
	var userTenant model.UserTenant
	err := DB.Where("tenant_id = ? AND user_id = ? AND status = ?", tenantID, userID, "1").
		First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// UserTenantDetail user tenant relationship with user details
type UserTenantDetail struct {
	ID             string     `gorm:"column:id" json:"id"`
	UserID         string     `gorm:"column:user_id" json:"user_id"`
	Status         *string    `gorm:"column:status" json:"status,omitempty"`
	Role           string     `gorm:"column:role" json:"role"`
	Nickname       string     `gorm:"column:nickname" json:"nickname"`
	Email          string     `gorm:"column:email" json:"email"`
	Avatar         *string    `gorm:"column:avatar" json:"avatar,omitempty"`
	IsAuthenticated string    `gorm:"column:is_authenticated" json:"is_authenticated"`
	IsActive       string     `gorm:"column:is_active" json:"is_active"`
	IsAnonymous    string     `gorm:"column:is_anonymous" json:"is_anonymous"`
	UserStatus     *string    `gorm:"column:user_status" json:"user_status,omitempty"`
	UpdateDate     *time.Time `gorm:"column:update_date" json:"update_date,omitempty"`
	IsSuperuser    *bool      `gorm:"column:is_superuser" json:"is_superuser,omitempty"`
}

// TenantDetail tenant with role and basic info
type TenantDetail struct {
	TenantID   string     `gorm:"column:tenant_id" json:"tenant_id"`
	Role       string     `gorm:"column:role" json:"role"`
	Nickname   string     `gorm:"column:nickname" json:"nickname"`
	Email      string     `gorm:"column:email" json:"email"`
	Avatar     *string    `gorm:"column:avatar" json:"avatar,omitempty"`
	UpdateDate *time.Time `gorm:"column:update_date" json:"update_date,omitempty"`
}