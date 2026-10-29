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

//go:build !ee

package handler

import (
	"errors"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

// billingQuotaUser resolves the caller that the plan quota applies to.
func billingQuotaUser(c *gin.Context) (*entity.User, bool) {
	user, code, message := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, message)
		return nil, false
	}
	return user, true
}

// billingEnforceApps admits every new application: the community edition has
// no plan quota.
func billingEnforceApps(_ *gin.Context, _ string) bool { return true }

// billingEnforceStorage admits every upload: the community edition has no
// storage quota.
func billingEnforceStorage(_ *gin.Context, _ string, _ int64) bool { return true }

// issueUserAccessToken mints the access token returned after a successful
// login.
func issueUserAccessToken(user *entity.User, secretKey string) (string, error) {
	if user == nil || user.AccessToken == nil {
		return "", errors.New("user access token is missing")
	}
	return common.DumpAccessToken(*user.AccessToken, "", secretKey)
}

// eeNotImplemented answers an enterprise-only endpoint in the community
// edition. The route stays registered so clients see "not implemented" rather
// than "not found".
func eeNotImplemented(c *gin.Context, feature string) {
	common.ErrorWithCode(c, common.CodeNotImplemented, feature+" not implemented")
}

func (h *SystemHandler) GetEnableAdmin(c *gin.Context) { eeNotImplemented(c, "EnableAdmin") }
func (h *SystemHandler) GetCores(c *gin.Context)       { eeNotImplemented(c, "Get cores") }
func (h *SystemHandler) SetCores(c *gin.Context)       { eeNotImplemented(c, "Set cores") }
func (h *SystemHandler) GetMemory(c *gin.Context)      { eeNotImplemented(c, "Get memory") }
func (h *SystemHandler) SetMemory(c *gin.Context)      { eeNotImplemented(c, "Set memory") }
func (h *SystemHandler) GetConcurrency(c *gin.Context) { eeNotImplemented(c, "Get concurrency") }
func (h *SystemHandler) SetConcurrency(c *gin.Context) { eeNotImplemented(c, "Set concurrency") }
func (h *SystemHandler) GetQuota(c *gin.Context)       { eeNotImplemented(c, "Get quota") }

func (h *UserHandler) IsAdmin(c *gin.Context)              { eeNotImplemented(c, "IsAdmin") }
func (h *UserHandler) GetMeta(c *gin.Context)              { eeNotImplemented(c, "GetMeta") }
func (h *ProviderHandler) CreateModelGroup(c *gin.Context) { eeNotImplemented(c, "CreateModelGroup") }
func (h *ProviderHandler) DeleteModelGroup(c *gin.Context) { eeNotImplemented(c, "DeleteModelGroup") }
func (h *ProviderHandler) ShowModelGroup(c *gin.Context)   { eeNotImplemented(c, "ShowModelGroup") }
func (h *ProviderHandler) AddModelToGroup(c *gin.Context)  { eeNotImplemented(c, "AddModelToGroup") }
func (h *ProviderHandler) RemoveModelFromGroup(c *gin.Context) {
	eeNotImplemented(c, "RemoveModelFromGroup")
}

func (h *StatsHandler) GetModelStats(c *gin.Context) { eeNotImplemented(c, "GetModelStats") }
func (h *StatsHandler) GetGroupStats(c *gin.Context) { eeNotImplemented(c, "GetGroupStats") }
