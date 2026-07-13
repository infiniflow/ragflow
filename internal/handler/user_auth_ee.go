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

	"github.com/gin-gonic/gin"
)

func (h *UserHandler) OAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "OAuthCallback not implemented")
}

func (h *UserHandler) GitHubAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "GitHubAuthCallback not implemented")
}

func (h *UserHandler) LarkAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "LarkAuthCallback not implemented")
}

func (h *UserHandler) ICBCAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "ICBCAuthCallback not implemented")
}

func (h *UserHandler) AzureAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "AzureAuthCallback not implemented")
}

func (h *UserHandler) AzureAuthLogin(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "AzureAuthLogin not implemented")
}

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
