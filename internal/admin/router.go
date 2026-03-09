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

	"ragflow/internal/handler"
)

// Router admin router
type Router struct {
	handler     *Handler
	userHandler *handler.UserHandler
}

// NewRouter create admin router
func NewRouter(handler *Handler) *Router {
	return &Router{
		handler: handler,
	}
}

// Setup setup routes
func (r *Router) Setup(engine *gin.Engine) {
	// Health check
	engine.GET("/health", r.handler.Health)

	// Admin API routes with prefix /api/v1/admin
	admin := engine.Group("/api/v1/admin")
	{
		// Public routes
		admin.GET("/ping", r.handler.Ping)
		admin.POST("/login", r.handler.Login)

		admin.POST("/reports", r.handler.Reports)

		// Protected routes
		protected := admin.Group("")
		protected.Use(r.handler.AuthMiddleware())
		{
			// Auth
			protected.GET("/auth", r.handler.AuthCheck)
			protected.GET("/logout", r.handler.Logout)

			// User management
			protected.GET("/users", r.handler.ListUsers)
			protected.POST("/users", r.handler.CreateUser)
			protected.GET("/users/:username", r.handler.GetUser)
			protected.DELETE("/users/:username", r.handler.DeleteUser)
			protected.PUT("/users/:username/password", r.handler.ChangePassword)
			protected.PUT("/users/:username/activate", r.handler.UpdateUserActivateStatus)
			protected.PUT("/users/:username/admin", r.handler.GrantAdmin)
			protected.DELETE("/users/:username/admin", r.handler.RevokeAdmin)
			protected.GET("/users/:username/datasets", r.handler.GetUserDatasets)
			protected.GET("/users/:username/agents", r.handler.GetUserAgents)

			// API Keys
			protected.GET("/users/:username/keys", r.handler.GetUserAPIKeys)
			protected.POST("/users/:username/keys", r.handler.GenerateUserAPIKey)
			protected.DELETE("/users/:username/keys/:key", r.handler.DeleteUserAPIKey)

			// Role management
			protected.GET("/roles", r.handler.ListRoles)
			protected.POST("/roles", r.handler.CreateRole)
			protected.GET("/roles/:role_name", r.handler.GetRole)
			protected.PUT("/roles/:role_name", r.handler.UpdateRole)
			protected.DELETE("/roles/:role_name", r.handler.DeleteRole)
			protected.GET("/roles/:role_name/permission", r.handler.GetRolePermission)
			protected.POST("/roles/:role_name/permission", r.handler.GrantRolePermission)
			protected.DELETE("/roles/:role_name/permission", r.handler.RevokeRolePermission)

			// User roles and permissions
			protected.PUT("/users/:username/role", r.handler.UpdateUserRole)
			protected.GET("/users/:username/permission", r.handler.GetUserPermission)

			// Service management
			protected.GET("/services", r.handler.GetServices)
			protected.GET("/service_types/:service_type", r.handler.GetServicesByType)
			protected.GET("/services/:service_id", r.handler.GetService)
			protected.DELETE("/services/:service_id", r.handler.ShutdownService)
			protected.PUT("/services/:service_id", r.handler.RestartService)

			// Variables/Settings
			protected.GET("/variables", r.handler.GetVariables)
			protected.PUT("/variables", r.handler.SetVariable)

			// Configs
			protected.GET("/configs", r.handler.GetConfigs)

			// Environments
			protected.GET("/environments", r.handler.GetEnvironments)

			// Version
			protected.GET("/version", r.handler.GetVersion)

			// Sandbox
			protected.GET("/sandbox/providers", r.handler.ListSandboxProviders)
			protected.GET("/sandbox/providers/:provider_id/schema", r.handler.GetSandboxProviderSchema)
			protected.GET("/sandbox/config", r.handler.GetSandboxConfig)
			protected.POST("/sandbox/config", r.handler.SetSandboxConfig)
			protected.POST("/sandbox/test", r.handler.TestSandboxConnection)
		}
	}

	// Handle undefined routes
	engine.NoRoute(r.handler.HandleNoRoute)
}
