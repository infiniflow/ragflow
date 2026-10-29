//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package dao

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/entity"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInvalidUserListQuery identifies unsupported user-list filters or sorting.
var ErrInvalidUserListQuery = errors.New("invalid user list query")

// UserDAO user data access object
type UserDAO struct{}

// NewUserDAO create user DAO
func NewUserDAO() *UserDAO {
	return &UserDAO{}
}

// Create user
func (dao *UserDAO) Create(ctx context.Context, db *gorm.DB, user *entity.User) error {
	return db.WithContext(ctx).Create(user).Error
}

// GetByID get user by ID
func (dao *UserDAO) GetByID(ctx context.Context, db *gorm.DB, id uint) (*entity.User, error) {
	var user entity.User
	err := db.WithContext(ctx).Take(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByIDString gets a user by its string ID.
func (dao *UserDAO) GetByIDString(ctx context.Context, db *gorm.DB, id string) (*entity.User, error) {
	var user entity.User
	err := db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (dao *UserDAO) GetByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (*entity.User, error) {
	var user entity.User
	err := db.WithContext(ctx).Where("id = ?", tenantID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetNicknameByID returns a user's nickname by string id.
func (dao *UserDAO) GetNicknameByID(ctx context.Context, db *gorm.DB, id string) (string, error) {
	var nickname string
	err := db.WithContext(ctx).
		Model(&entity.User{}).
		Where("id = ?", id).
		Select("nickname").
		Scan(&nickname).Error
	return nickname, err
}

// GetNicknamesByIDs resolves nicknames for many users in one batched
// lookup, keyed by user id; ids without a matching user row are absent.
func (dao *UserDAO) GetNicknamesByIDs(ctx context.Context, db *gorm.DB, ids []string) (map[string]string, error) {
	nicknames := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return nicknames, nil
	}
	var rows []struct {
		ID       string
		Nickname string
	}
	err := db.WithContext(ctx).
		Model(&entity.User{}).
		Select("id, nickname").
		Where("id IN ?", ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		nicknames[r.ID] = r.Nickname
	}
	return nicknames, nil
}

// GetByEmail get user by email
func (dao *UserDAO) GetByEmail(ctx context.Context, db *gorm.DB, email string) (*entity.User, error) {
	var user entity.User
	query := db.WithContext(ctx).Where("email = ?", email)
	err := query.First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByAccessToken get user by access token
func (dao *UserDAO) GetByAccessToken(ctx context.Context, db *gorm.DB, token string) (*entity.User, error) {
	var user entity.User
	err := db.WithContext(ctx).Where("access_token = ?", token).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update update user
func (dao *UserDAO) Update(ctx context.Context, db *gorm.DB, user *entity.User) error {
	return db.WithContext(ctx).Save(user).Error
}

// UpdateAccessToken update user's access token
func (dao *UserDAO) UpdateAccessToken(ctx context.Context, db *gorm.DB, user *entity.User, token string) error {
	return db.WithContext(ctx).Model(user).Update("access_token", token).Error
}

// List returns non-deleted users and their filtered total. Conditions on db are
// applied before counting and pagination, so callers can add further filters.
func (dao *UserDAO) List(ctx context.Context, db *gorm.DB, offset, limit int, name, status, sort, orderBy string) ([]*entity.User, int64, error) {
	query, err := userListQuery(ctx, db, name, status)
	if err != nil {
		return nil, 0, err
	}
	order, err := userListOrder(sort, orderBy)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order(order).
		Order(clause.OrderByColumn{Column: clause.Column{Table: "user", Name: "id"}})
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	users := make([]*entity.User, 0)
	err = query.Select("user.*").Find(&users).Error
	return users, total, err
}

func userListQuery(ctx context.Context, db *gorm.DB, keyword, status string) (*gorm.DB, error) {
	query := db.WithContext(ctx).Model(&entity.User{}).
		Where(clause.Neq{Column: clause.Column{Table: "user", Name: "status"}, Value: "0"})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(keyword)) + "%"
		query = query.Where("(LOWER(?) LIKE ? ESCAPE '!' OR LOWER(?) LIKE ? ESCAPE '!')",
			clause.Column{Table: "user", Name: "email"}, pattern,
			clause.Column{Table: "user", Name: "nickname"}, pattern)
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
	case "active":
		query = query.Where(clause.Eq{Column: clause.Column{Table: "user", Name: "is_active"}, Value: "1"})
	case "inactive":
		query = query.Where(clause.Eq{Column: clause.Column{Table: "user", Name: "is_active"}, Value: "0"})
	default:
		return nil, fmt.Errorf("%w: status must be active or inactive", ErrInvalidUserListQuery)
	}
	return query, nil
}

func userListOrder(direction, field string) (clause.OrderByColumn, error) {
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		field = "create_date"
	}
	switch field {
	case "create_date", "last_login_time":
	default:
		return clause.OrderByColumn{}, fmt.Errorf("%w: unsupported order field %q", ErrInvalidUserListQuery, field)
	}
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction == "" {
		direction = "desc"
	}
	if direction != "asc" && direction != "desc" {
		return clause.OrderByColumn{}, fmt.Errorf("%w: sort must be asc or desc", ErrInvalidUserListQuery)
	}
	return clause.OrderByColumn{Column: clause.Column{Table: "user", Name: field}, Desc: direction == "desc"}, nil
}

// Delete delete user
func (dao *UserDAO) Delete(ctx context.Context, db *gorm.DB, id uint) error {
	return db.WithContext(ctx).Delete(&entity.User{}, id).Error
}

// HardDelete hard delete user by string ID
func (dao *UserDAO) HardDelete(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&entity.User{}).Error
}

// ListByEmail list users by email (only active users with status != "0")
// Returns all users matching the given email address
func (dao *UserDAO) ListByEmail(ctx context.Context, db *gorm.DB, email string) ([]*entity.User, error) {
	var users []*entity.User
	err := db.WithContext(ctx).Where("email = ?", email).Find(&users).Error
	return users, err
}
