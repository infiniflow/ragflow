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
	"github.com/gin-gonic/gin"
)

// Router admin router
type Router struct {
	handler *Handler
}

// NewRouter create admin router
func NewRouter(handler *Handler) *Router {
	return &Router{handler: handler}
}

// Setup setup routes
func (r *Router) Setup(engine *gin.Engine) {
	// Health check
	engine.GET("/health", r.handler.Health)

	// Admin API routes
	admin := engine.Group("/admin")
	{
		// Auth
		admin.POST("/login", r.handler.Login)

		// Protected routes
		protected := admin.Group("")
		protected.Use(r.handler.AuthMiddleware())
		{
			// User management
			protected.GET("/users", r.handler.ListUsers)
			protected.GET("/users/:id", r.handler.GetUser)
			protected.PUT("/users/:id", r.handler.UpdateUser)
			protected.DELETE("/users/:id", r.handler.DeleteUser)

			// System config
			protected.GET("/config", r.handler.GetConfig)
			protected.PUT("/config", r.handler.UpdateConfig)

			// System status
			protected.GET("/status", r.handler.GetStatus)
		}
	}

	// Handle undefined routes
	engine.NoRoute(r.handler.HandleNoRoute)
}
