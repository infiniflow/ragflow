package dao

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/entity"
	"ragflow/internal/utility"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDocumentCleanupClaimHeld = errors.New("document cleanup claim is held")
	ErrDocumentCleanupClaimLost = errors.New("document cleanup claim was lost")
)

// DocumentCleanupClaimDAO persists fencing claims for document cleanup.
type DocumentCleanupClaimDAO struct{}

// NewDocumentCleanupClaimDAO creates a document cleanup claim DAO.
func NewDocumentCleanupClaimDAO() *DocumentCleanupClaimDAO {
	return &DocumentCleanupClaimDAO{}
}

// Acquire creates a new claim or takes over one whose lease and grace period
// have both elapsed. The caller must hold the corresponding document row lock.
func (dao *DocumentCleanupClaimDAO) Acquire(ctx context.Context, db *gorm.DB, documentID, owner string, now, lease, grace int64) (*entity.DocumentCleanupClaim, error) {
	if documentID == "" || owner == "" || lease <= 0 || grace < 0 {
		return nil, fmt.Errorf("invalid document cleanup claim")
	}

	var existing entity.DocumentCleanupClaim
	err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "document_id = ?", documentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		claim := &entity.DocumentCleanupClaim{
			DocumentID: documentID,
			Token:      utility.GenerateToken(),
			Owner:      owner,
			ExpiresAt:  now + lease,
			UpdateTime: now,
		}
		if err = db.WithContext(ctx).Create(claim).Error; err != nil {
			return nil, err
		}
		return claim, nil
	}
	if err != nil {
		return nil, err
	}
	if existing.ExpiresAt+grace > now {
		return nil, ErrDocumentCleanupClaimHeld
	}

	existing.Token = utility.GenerateToken()
	existing.Owner = owner
	existing.ExpiresAt = now + lease
	existing.UpdateTime = now
	if err = db.WithContext(ctx).Model(&entity.DocumentCleanupClaim{}).
		Where("document_id = ?", documentID).
		Updates(map[string]any{
			"token":       existing.Token,
			"owner":       existing.Owner,
			"expires_at":  existing.ExpiresAt,
			"update_time": existing.UpdateTime,
		}).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}

// Renew extends a claim only when the fencing token still owns it.
func (dao *DocumentCleanupClaimDAO) Renew(ctx context.Context, db *gorm.DB, documentID, token string, now, lease int64) error {
	if documentID == "" || token == "" || lease <= 0 {
		return ErrDocumentCleanupClaimLost
	}
	result := db.WithContext(ctx).Model(&entity.DocumentCleanupClaim{}).
		Where("document_id = ? AND token = ?", documentID, token).
		Updates(map[string]any{"expires_at": now + lease, "update_time": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrDocumentCleanupClaimLost
	}
	return nil
}

// Validate reports whether token owns an unexpired claim.
func (dao *DocumentCleanupClaimDAO) Validate(ctx context.Context, db *gorm.DB, documentID, token string, now int64) bool {
	var count int64
	if err := db.WithContext(ctx).Model(&entity.DocumentCleanupClaim{}).
		Where("document_id = ? AND token = ? AND expires_at > ?", documentID, token, now).
		Count(&count).Error; err != nil {
		return false
	}
	return count == 1
}

// GetActive returns the currently unexpired claim for a document.
func (dao *DocumentCleanupClaimDAO) GetActive(ctx context.Context, db *gorm.DB, documentID string, now int64) (*entity.DocumentCleanupClaim, error) {
	return dao.GetBlocking(ctx, db, documentID, now, 0)
}

// GetBlocking returns a claim that still blocks a new owner, including the
// takeover-grace interval after its lease expires. Callers that are about to
// create a new run must use this method rather than GetActive: an expired
// claim may still have an external cleanup batch in flight until grace ends.
func (dao *DocumentCleanupClaimDAO) GetBlocking(ctx context.Context, db *gorm.DB, documentID string, now, grace int64) (*entity.DocumentCleanupClaim, error) {
	if documentID == "" || grace < 0 {
		return nil, fmt.Errorf("invalid document cleanup claim lookup")
	}
	var claim entity.DocumentCleanupClaim
	err := db.WithContext(ctx).Where("document_id = ? AND expires_at + ? > ?", documentID, grace, now).First(&claim).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

// CurrentUnixTime reads the database clock in seconds so lease comparisons do
// not depend on the clocks of individual API or worker processes.
func CurrentUnixTime(ctx context.Context, db *gorm.DB) (int64, error) {
	query := "SELECT UNIX_TIMESTAMP()"
	switch db.Dialector.Name() {
	case "sqlite":
		query = "SELECT CAST(strftime('%s', 'now') AS INTEGER)"
	case "postgres":
		query = "SELECT CAST(EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) AS BIGINT)"
	}
	var now int64
	if err := db.WithContext(ctx).Raw(query).Scan(&now).Error; err != nil {
		return 0, err
	}
	return now, nil
}

// Release removes a claim only when token still owns it.
func (dao *DocumentCleanupClaimDAO) Release(ctx context.Context, db *gorm.DB, documentID, token string) (bool, error) {
	result := db.WithContext(ctx).Where("document_id = ? AND token = ?", documentID, token).Delete(&entity.DocumentCleanupClaim{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
