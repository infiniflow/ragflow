package dao

import (
	"context"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// OntologyActionContractEditDAO persists the history of manual contract edits.
type OntologyActionContractEditDAO struct{}

// NewOntologyActionContractEditDAO creates the history DAO.
func NewOntologyActionContractEditDAO() *OntologyActionContractEditDAO {
	return &OntologyActionContractEditDAO{}
}

// Create appends one edit record.
func (dao *OntologyActionContractEditDAO) Create(ctx context.Context, db *gorm.DB, row *entity.OntologyActionContractEdit) error {
	return db.WithContext(ctx).Create(row).Error
}

// ListByAction returns the newest edits of one action, paged.
//
// It is keyed by the action name rather than the row id: the id changes on every
// recompile, and the history of "what may this action do" outlives one compiled
// row. The name is what an operator bound and what a question refers to.
func (dao *OntologyActionContractEditDAO) ListByAction(
	ctx context.Context, db *gorm.DB, tenantID, templateID, actionName string, offset, limit int,
) ([]entity.OntologyActionContractEdit, int64, error) {
	query := db.WithContext(ctx).Model(&entity.OntologyActionContractEdit{}).
		Where("tenant_id = ? AND template_id = ? AND action_name = ?", tenantID, templateID, actionName)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []entity.OntologyActionContractEdit{}
	err := query.Order("create_time DESC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}
