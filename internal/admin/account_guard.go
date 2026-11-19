package admin

import (
	"context"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type adminActorKey struct{}

type adminActor struct {
	ID          string
	AccessToken string
}

func activeSuperuser(user *entity.User) bool {
	return user.IsSuperuser != nil && *user.IsSuperuser &&
		user.IsActive == "1" && user.Status != nil && *user.Status == "1"
}

// updateAccount serializes administrator changes before locking the target user.
// A locking read of all superusers prevents concurrent requests from removing
// the final active administrator under MySQL's repeatable-read isolation.
func (s *Service) updateAccount(ctx context.Context, email, action string, update func(*gorm.DB, *entity.User) error) error {
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var administrators []entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("is_superuser = ?", true).Order("id").Find(&administrators).Error; err != nil {
			return err
		}

		actor, authenticated := ctx.Value(adminActorKey{}).(adminActor)
		activeCount := 0
		actorValid := false
		for i := range administrators {
			admin := &administrators[i]
			if activeSuperuser(admin) {
				activeCount++
				actorValid = actorValid || (admin.ID == actor.ID &&
					admin.AccessToken != nil && *admin.AccessToken == actor.AccessToken)
			}
		}
		if authenticated && !actorValid {
			return common.NewCodedError(common.CodeUnauthorized, "Administrator session is no longer valid")
		}

		var users []entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("email = ?", email).Limit(2).Find(&users).Error; err != nil {
			return err
		}
		if len(users) == 0 {
			return common.NewCodedError(common.CodeNotFound, "User not found")
		}
		if len(users) != 1 {
			return common.NewCodedError(common.CodeConflict, "Multiple users match this account")
		}
		user := &users[0]
		if action == "disable" || action == "revoke_admin" {
			if authenticated && actor.ID == user.ID {
				return common.NewCodedError(common.CodeConflict, "You cannot disable your own account or revoke your own administrator privileges")
			}
			if activeSuperuser(user) && activeCount <= 1 {
				return common.NewCodedError(common.CodeConflict, "At least one active superuser must remain")
			}
		}
		return update(tx, user)
	})
}
