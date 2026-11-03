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

package router

import "github.com/gin-gonic/gin"

// SetupEERouter adds no enterprise middleware in the community edition.
func SetupEERouter(engine *gin.Engine) {}

// RegisterEENoAuthRouter keeps the unauthenticated enterprise endpoints
// registered so clients see "not implemented" instead of "not found".
// RegisterEEProtectedRouter registers no authenticated enterprise routes in
// the community edition.
// RegisterEENoAuthRouter registers no enterprise auth routes in the community
// edition.
func RegisterEENoAuthRouter(_ *gin.Engine, _ *gin.RouterGroup, _ *Router) {}

func RegisterEEProtectedRouter(_ *gin.RouterGroup, _ *Router) {}

// RegisterEEAuthRouter registers no enterprise auth routes in the community
// edition.
func RegisterEEAuthRouter(_ *gin.RouterGroup, _ *Router) {}

// allowAll is the community gate for a feature toggle: every request passes.
func allowAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func ToggleChat() gin.HandlerFunc    { return allowAll() }
func ToggleDataset() gin.HandlerFunc { return allowAll() }
func ToggleSearch() gin.HandlerFunc  { return allowAll() }
func ToggleFile() gin.HandlerFunc    { return allowAll() }
func ToggleMemory() gin.HandlerFunc  { return allowAll() }
func ToggleModel() gin.HandlerFunc   { return allowAll() }
