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
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

func (h *UserHandler) Captcha(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Captcha not implemented")
}

func (h *UserHandler) SendOTP(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "SendOTP not implemented")
}

func (h *UserHandler) VerifyOTP(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "VerifyOTP not implemented")
}

func (h *UserHandler) IsAdmin(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "IsAdmin not implemented")
}

func (h *UserHandler) GetMeta(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "GetMeta not implemented")
}

func issueUserAccessToken(user *entity.User, secretKey string) (string, error) {
	if user == nil || user.AccessToken == nil {
		return "", fmt.Errorf("user access token is missing")
	}
	return common.DumpAccessToken(*user.AccessToken, "", secretKey)
}
