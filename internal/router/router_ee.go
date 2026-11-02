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

package router

import (
	"github.com/gin-gonic/gin"
)

func SetupEERouter(engine *gin.Engine) {
}

func RegisterEENoAuthRouter(apiNoAuth *gin.RouterGroup, r *Router) {
	// For EE
	apiNoAuth.POST("/auth/register/captcha", r.userHandler.Captcha)
	apiNoAuth.POST("/auth/register/otp", r.userHandler.SendOTP)
	apiNoAuth.POST("/auth/register/otp/verify", r.userHandler.VerifyOTP)
}

func RegisterEEProtectedRouter(_ *gin.RouterGroup, _ *Router) {}

func ToggleWorkflow() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleChat() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleFile() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleSearch() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleModel() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleUserInTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleGroup() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleEvaluation() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleDataset() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleMemory() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
