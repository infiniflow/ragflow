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

package handler

import (
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

// orderTermsFromQuery reads the `sort` parameter, a comma separated list of
// `column:direction` terms such as `name:asc,create_time:desc`. It takes
// precedence over the older `orderby` and `desc` pair, which still works on its
// own and is what an absent or unusable `sort` falls back to. A term naming a
// column the entity does not order by is dropped rather than rejected, so a
// caller that already sends an unknown name keeps the result it has today.
func orderTermsFromQuery(c *gin.Context, orderby string, desc bool) []dao.OrderTerm {
	if terms := dao.ParseOrderTerms(c.Query("sort")); len(terms) > 0 {
		return terms
	}
	return []dao.OrderTerm{{Column: orderby, Desc: desc}}
}

func GetUser(c *gin.Context) (*entity.User, common.ErrorCode, string) {
	userAny, exist := c.Get("user")
	if !exist {
		return nil, common.CodeUnauthorized, "User not found"
	}

	user, ok := userAny.(*entity.User)
	if !ok {
		return nil, common.CodeUnauthorized, "User not found"
	}
	return user, common.CodeSuccess, ""
}
