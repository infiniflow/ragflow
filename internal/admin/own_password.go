package admin

import (
	"context"
	"encoding/base64"
	"errors"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Native browser and CLI clients hash the Base64 text inside the RSA envelope.
// Validate its decoded characters without changing the value used for hashing.
func validateAdminPassword(password string) error {
	if decoded, err := base64.StdEncoding.DecodeString(password); err == nil {
		if !utf8.Valid(decoded) {
			return common.NewCodedError(common.CodeBadRequest, "Invalid password encoding")
		}
		password = string(decoded)
	}
	if length := utf8.RuneCountInString(password); length < 8 || length > 128 {
		return common.NewCodedError(common.CodeBadRequest, "Password must contain 8 to 128 characters")
	}
	return nil
}

// ChangeOwnPassword verifies the existing password against the locked account.
// ChangeOwnPassword verifies the existing password and invalidates old sessions atomically.
func (s *Service) ChangeOwnPassword(ctx context.Context, actor *entity.User, oldPassword, newPassword string) error {
	if actor == nil || actor.AccessToken == nil {
		return common.NewCodedError(common.CodeUnauthorized, "Administrator authentication required")
	}
	oldPlain, err := common.DecryptPassword(oldPassword)
	if err != nil {
		return common.NewCodedError(common.CodeBadRequest, "Invalid encrypted current password")
	}
	newPlain, err := common.DecryptPassword(newPassword)
	if err != nil {
		return common.NewCodedError(common.CodeBadRequest, "Invalid encrypted new password")
	}
	if err := validateAdminPassword(newPlain); err != nil {
		return err
	}
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&user, "id = ?", actor.ID).Error
		if err != nil {
			return err
		}
		if user.IsSuperuser == nil || !*user.IsSuperuser || user.IsActive != "1" ||
			user.Status == nil || *user.Status != "1" || user.AccessToken == nil ||
			*user.AccessToken != *actor.AccessToken {
			return common.NewCodedError(common.CodeUnauthorized, "Administrator session is no longer valid")
		}
		if user.Password == nil || !common.CheckWerkzeugPassword(oldPlain, *user.Password) {
			return common.NewCodedError(common.CodeForbidden, "Current password is incorrect")
		}
		if common.CheckWerkzeugPassword(newPlain, *user.Password) {
			return common.NewCodedError(common.CodeBadRequest, "New password must differ from the current password")
		}
		hash, err := common.GenerateWerkzeugPasswordHash(newPlain)
		if err != nil {
			return err
		}
		return tx.Model(&user).Updates(map[string]interface{}{
			"password": hash, "access_token": "INVALID_" + common.GenerateToken(),
		}).Error
	})
}

func (h *Handler) ChangeOwnPassword(c *gin.Context) {
	var request struct {
		OldPassword string `json:"old_password" binding:"required,max=4096"`
		NewPassword string `json:"new_password" binding:"required,max=4096"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.RespondError(c, common.CodeBadRequest, "Current and new passwords are required")
		return
	}
	value, _ := c.Get("user")
	user, _ := value.(*entity.User)
	if err := h.service.ChangeOwnPassword(c.Request.Context(), user, request.OldPassword, request.NewPassword); err != nil {
		var coded *common.CodedError
		if errors.As(err, &coded) {
			common.RespondError(c, coded.Code, coded.Message)
		} else {
			common.Error("Administrator password change failed", err)
			common.RespondError(c, common.CodeServerError, "Could not change password")
		}
		return
	}
	common.RespondSuccess(c, nil, "Password changed; sign in again")
}
