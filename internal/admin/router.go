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

		//admin.POST("/ingestion/tasks", r.handler.StartIngestionTask)
		//admin.DELETE("/ingestion", r.handler.CancelIngestionTask) // cancel ingestion
		//admin.GET("/ingestion/tasks", r.handler.ListIngestionTasks)

		// Protected routes
		protected := admin.Group("")
		protected.Use(r.handler.AuthMiddleware())
		{

			protected.POST("/logout", r.handler.Logout)
			// Auth
			protected.GET("/auth", r.handler.AuthCheck)

			// User management
			protected.GET("/users", r.handler.ListUsers)
			protected.POST("/users", r.handler.CreateUser)
			protected.GET("/users/:username", r.handler.GetUser)
			protected.DELETE("/users/:username", r.handler.DeleteUser)
			protected.PUT("/users/:username/password", r.handler.ChangePassword)
			protected.PUT("/users/:username/activate", r.handler.UpdateUserActivateStatus)
			protected.PUT("/users/:username/admin", r.handler.GrantAdmin)
			protected.DELETE("/users/:username/admin", r.handler.RevokeAdmin)

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

			// Log level
			protected.GET("/log_level", r.handler.GetLogLevel)
			protected.PUT("/log_level", r.handler.SetLogLevel)

			provider := protected.Group("/providers")
			{
				provider.GET("/", r.handler.ListProviders)
				provider.GET("/:provider_name", r.handler.ShowProvider)
				provider.GET("/:provider_name/models", r.handler.ListModels)
				provider.GET("/:provider_name/models/:model_name", r.handler.ShowModel)
			}

			queue := protected.Group("/queue")
			{
				queue.GET("/", r.handler.ShowMessageQueue)
				queue.POST("/messages", r.handler.PublishMessageToQueue)
				queue.GET("/messages", r.handler.ListMessagesFromQueue)
				queue.PUT("/messages", r.handler.PullMessageFromQueue)
			}

			protected.GET("/ingestors", r.handler.ListIngestors)
			protected.DELETE("/ingestors", r.handler.ShutdownIngestor)

			// Sandbox
			protected.GET("/sandbox/providers", r.handler.ListSandboxProviders)
			protected.GET("/sandbox/providers/:provider_id/schema", r.handler.GetSandboxProviderSchema)
			protected.GET("/sandbox/config", r.handler.GetSandboxConfig)
			protected.POST("/sandbox/config", r.handler.SetSandboxConfig)
			protected.POST("/sandbox/test", r.handler.TestSandboxConnection)

			// For enterprise edition
			protected.GET("/users/:username/activity", r.handler.ShowUserActivity)
			protected.GET("/users/:username/dataset", r.handler.ShowUserDatasetSummary)
			protected.GET("/users/:username/summary", r.handler.ShowUserSummary)
			protected.GET("/users/:username/storage", r.handler.ShowUserStorage)
			protected.GET("/users/:username/quota", r.handler.ShowUserQuota)
			protected.GET("/users/:username/index", r.handler.ShowUserIndex)
			protected.PUT("/users/:username/role", r.handler.UpdateUserRole)
			protected.GET("/users/:username/permission", r.handler.ShowUserPermission)
			protected.GET("/users/:username/datasets", r.handler.ListUserDatasets)
			protected.GET("/users/:username/agents", r.handler.ListUserAgents)
			protected.GET("/users/:username/chats", r.handler.ListUserChats)
			protected.GET("/users/:username/searches", r.handler.ListUserSearches)
			protected.GET("/users/:username/models", r.handler.ListUserModels)
			protected.GET("/users/:username/files", r.handler.ListUserFiles)
			protected.GET("/users/summary", r.handler.ShowUsersSummary)
			protected.GET("/users/activity", r.handler.ShowUsersActivity)
			protected.GET("/users/reports", r.handler.ListUsersReports)
			protected.GET("/users/storage", r.handler.ListUsersStorage)
			protected.GET("/users/documents", r.handler.ListUsersDocuments)
			protected.GET("/users/index", r.handler.ListUsersIndex)
			protected.GET("/users/quota", r.handler.ListUsersQuota)
			protected.GET("/users/quota/summary", r.handler.ShowUsersQuotaSummary)
			protected.GET("/ingestion/tasks/summary", r.handler.ShowIngestionTasksSummary)
			protected.GET("/data/summary", r.handler.ShowDataSummary)
			protected.GET("/data/orphan", r.handler.ShowDataOrphan)
			protected.GET("/data/storage", r.handler.ShowDataStorage)
			protected.GET("/data/index", r.handler.ShowDataIndex)
			protected.DELETE("/data/orphan", r.handler.PurgeOrphanData)
			protected.DELETE("/users/:username/data", r.handler.PurgeUserData)
			protected.DELETE("/users/data", r.handler.PurgeUsersData)

			// API Keys
			protected.POST("/users/:username/keys", r.handler.CreateUserAPIKey)
			protected.DELETE("/users/:username/keys/:key", r.handler.DeleteUserAPIKey)
			protected.GET("/users/:username/keys", r.handler.ListUserAPIKeys)

			protected.GET("/users/:username/tokens", r.handler.ListUserAPITokens)
			//protected.POST("/users/:username/keys", r.handler.GenerateUserAPIToken)
			protected.POST("/users/:username/tokens", r.handler.GenerateUserAPIToken)
			protected.DELETE("/users/:username/tokens/:token", r.handler.DeleteUserAPIToken)

			// Role management
			protected.GET("/roles", r.handler.ListRoles)
			protected.POST("/roles", r.handler.CreateRole)
			protected.GET("/roles/:role_name", r.handler.ShowRole)
			protected.PUT("/roles/:role_name", r.handler.UpdateRole)
			protected.DELETE("/roles/:role_name", r.handler.DeleteRole)
			protected.GET("/roles/:role_name/permission", r.handler.GetRolePermission)
			protected.POST("/roles/:role_name/permission", r.handler.GrantRolePermission)
			protected.DELETE("/roles/:role_name/permission", r.handler.RevokeRolePermission)
			protected.GET("/roles/resource", r.handler.ListResources)

			// Models
			protected.GET("/all-models", r.handler.ListModelsOrShowModel)

			// License
			protected.GET("/system/fingerprint", r.handler.GetSystemFingerprint)
			protected.POST("/system/license", r.handler.SetSystemLicense)
			protected.GET("/system/license", r.handler.ShowSystemLicense)
			protected.PUT("/system/license/config", r.handler.UpdateSystemLicenseConfig)

			// Fingerprint
			protected.GET("/fingerprint", r.handler.GetFingerprint)
			// License
			protected.POST("/license", r.handler.SetLicense)
			protected.POST("/license/config", r.handler.UpdateLicenseConfig)
			protected.GET("/license", r.handler.ShowLicense)

			// Ingestion tasks
			protected.DELETE("/ingestion/tasks", r.handler.RemoveIngestionTasks)
			protected.PUT("/ingestion/tasks", r.handler.StopIngestionTasks)
			protected.GET("/ingestion/tasks", r.handler.ListIngestionTasks)
		}
	}

	// Handle undefined routes
	engine.NoRoute(r.handler.HandleNoRoute)
}
