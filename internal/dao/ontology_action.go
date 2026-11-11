package dao

import (
	"context"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// OntologyActionBindingDAO persists action→tool mappings.
type OntologyActionBindingDAO struct{}

// NewOntologyActionBindingDAO creates the binding DAO.
func NewOntologyActionBindingDAO() *OntologyActionBindingDAO {
	return &OntologyActionBindingDAO{}
}

// ListByTemplate returns every binding of a model.
func (dao *OntologyActionBindingDAO) ListByTemplate(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]entity.OntologyActionBinding, error) {
	var rows []entity.OntologyActionBinding
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ?", tenantID, templateID).
		Order("action_name ASC").
		Find(&rows).Error
	return rows, err
}

// FindByAction returns the binding of one action, or nil when unbound.
func (dao *OntologyActionBindingDAO) FindByAction(ctx context.Context, db *gorm.DB, tenantID, templateID, actionName string) (*entity.OntologyActionBinding, error) {
	var rows []entity.OntologyActionBinding
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ? AND action_name = ?", tenantID, templateID, actionName).
		Limit(1).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// Save inserts or replaces one binding.
//
// One action has at most one binding: the unique index on
// (tenant_id, template_id, action_name) is what makes "which tool runs this"
// answerable, and an upsert keeps the write path from needing a transaction.
func (dao *OntologyActionBindingDAO) Save(ctx context.Context, db *gorm.DB, row *entity.OntologyActionBinding) error {
	return db.WithContext(ctx).Save(row).Error
}

// Delete removes one binding inside a tenant.
func (dao *OntologyActionBindingDAO) Delete(ctx context.Context, db *gorm.DB, tenantID, id string) error {
	return db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&entity.OntologyActionBinding{}).Error
}

// OntologyActionExecutionDAO persists the audit trail of action attempts.
type OntologyActionExecutionDAO struct{}

// NewOntologyActionExecutionDAO creates the audit DAO.
func NewOntologyActionExecutionDAO() *OntologyActionExecutionDAO {
	return &OntologyActionExecutionDAO{}
}

// Create appends one audit row.
func (dao *OntologyActionExecutionDAO) Create(ctx context.Context, db *gorm.DB, row *entity.OntologyActionExecution) error {
	return db.WithContext(ctx).Create(row).Error
}

// ListByTemplate returns the newest attempts of a model, paged.
func (dao *OntologyActionExecutionDAO) ListByTemplate(ctx context.Context, db *gorm.DB, tenantID, templateID string, offset, limit int) ([]entity.OntologyActionExecution, int64, error) {
	query := db.WithContext(ctx).Model(&entity.OntologyActionExecution{}).
		Where("tenant_id = ? AND template_id = ?", tenantID, templateID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []entity.OntologyActionExecution
	err := query.Order("create_time DESC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}
