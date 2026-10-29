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

package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/dao"
)

var errInvalidUserListQuery = errors.New("invalid user list query")

// ListUsers returns a page of users and the total number of users.
func (s *Service) ListUsers(ctx context.Context, pageIndex, pageSize int, name, status, sort, orderBy string) ([]map[string]interface{}, int64, error) {
	opts, err := parseUserListOptions(pageIndex, pageSize, name, status, sort, orderBy)
	if err != nil {
		return nil, 0, err
	}
	users, total, err := s.userDAO.List(ctx, dao.DB, opts)
	if err != nil {
		return nil, 0, err
	}

	result := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		result = append(result, map[string]interface{}{
			"email":        user.Email,
			"nickname":     user.Nickname,
			"create_date":  user.CreateTime,
			"is_active":    user.IsActive,
			"is_superuser": user.IsSuperuser,
		})
	}
	return result, total, nil
}

func parseUserListOptions(page, pageSize int, keyword, status, direction, field string) (dao.UserListOptions, error) {
	opts := dao.UserListOptions{Keyword: strings.TrimSpace(keyword), Limit: pageSize}
	if page < 1 || pageSize < 1 {
		return opts, fmt.Errorf("%w: page and page_size must be positive integers", errInvalidUserListQuery)
	}
	opts.Offset = (page - 1) * pageSize
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "":
	case "active", "inactive":
		active := status == "active"
		opts.Active = &active
	default:
		return opts, fmt.Errorf("%w: status must be active or inactive", errInvalidUserListQuery)
	}

	field = strings.ToLower(strings.TrimSpace(field))
	switch field {
	case "", "create_date":
		opts.OrderBy = dao.UserListByCreateDate
	case "last_login_time":
		opts.OrderBy = dao.UserListByLastLoginTime
	default:
		return opts, fmt.Errorf("%w: unsupported order field %q", errInvalidUserListQuery, field)
	}
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "", "desc":
		opts.Desc = true
	case "asc":
	default:
		return opts, fmt.Errorf("%w: sort must be asc or desc", errInvalidUserListQuery)
	}
	return opts, nil
}
