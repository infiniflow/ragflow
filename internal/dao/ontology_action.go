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

// ExecutionFilter narrows the audit trail. Zero values mean "no restriction",
// so the default query is the whole trail.
//
// The filters are applied in SQL rather than on the page: a reader who narrows
// to "refused" and gets the refusals of the current twenty rows has been shown
// the window, not the answer.
type ExecutionFilter struct {
	ActionName string
	Verdict    string
	Status     string
	Mode       string
	// FromMS/ToMS are unix milliseconds over create_time, matching BaseModel.
	FromMS int64
	ToMS   int64
	Offset int
	Limit  int
}

// ListByTemplate returns the newest attempts of a model, filtered and paged.
func (dao *OntologyActionExecutionDAO) ListByTemplate(
	ctx context.Context, db *gorm.DB, tenantID, templateID string, filter ExecutionFilter,
) ([]entity.OntologyActionExecution, int64, error) {
	query := db.WithContext(ctx).Model(&entity.OntologyActionExecution{}).
		Where("tenant_id = ? AND template_id = ?", tenantID, templateID)
	if filter.ActionName != "" {
		query = query.Where("action_name = ?", filter.ActionName)
	}
	if filter.Verdict != "" {
		query = query.Where("verdict = ?", filter.Verdict)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Mode != "" {
		query = query.Where("mode = ?", filter.Mode)
	}
	if filter.FromMS > 0 {
		query = query.Where("create_time >= ?", filter.FromMS)
	}
	if filter.ToMS > 0 {
		query = query.Where("create_time <= ?", filter.ToMS)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []entity.OntologyActionExecution{}
	err := query.Order("create_time DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Find(&rows).Error
	return rows, total, err
}
