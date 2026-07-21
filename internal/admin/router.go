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
	SetupEERouter(engine)

	// Healthz to get system health
	engine.GET("/healthz", r.handler.Healthz)
	engine.GET("/", r.handler.Live)
	engine.GET("/live", r.handler.Live)

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
			protected.POST("/services/:service_id", r.handler.StartService)

			// Variables/Settings
			protected.GET("/variables", r.handler.ListVariables)
			protected.PUT("/variables", r.handler.SetVariable)
			protected.GET("/variables/:var_name", r.handler.ShowVariable)

			// Configs
			protected.GET("/configs", r.handler.ListConfigs)
			// Log level
			protected.GET("/config/log", r.handler.GetLogLevel)
			protected.PUT("/config/log", r.handler.SetLogLevel)

			// Environments
			protected.GET("/environments", r.handler.ListEnvironments)

			// Version
			protected.GET("/version", r.handler.GetVersion)

			queue := protected.Group("/queue")
			{
				queue.GET("/", r.handler.ShowMessageQueue)
				queue.POST("/messages", r.handler.PublishMessageToQueue)
				queue.GET("/messages", r.handler.ListMessagesFromQueue)
				queue.PUT("/messages", r.handler.PullMessageFromQueue)
			}

			protected.GET("/store", r.handler.PingStore)
			protected.GET("/cache", r.handler.PingCache)
			protected.GET("/engine", r.handler.PingEngine)

			protected.GET("/ingestors", r.handler.ListIngestors)
			protected.DELETE("/ingestors", r.handler.ShutdownIngestor)

			// Sandbox
			protected.GET("/sandbox/providers", r.handler.ListSandboxProviders)
			protected.GET("/sandbox/providers/:provider_id/schema", r.handler.GetSandboxProviderSchema)
			protected.GET("/sandbox/config", r.handler.GetSandboxConfig)
			protected.POST("/sandbox/config", r.handler.SetSandboxConfig)
			protected.POST("/sandbox/test", r.handler.TestSandboxConnection)

			protected.GET("/all-models", r.handler.ListAllModels)
			protected.GET("/all-models/:model_name", r.handler.ShowModel)

			// Ingestion tasks
			protected.DELETE("/ingestion/tasks", r.handler.RemoveIngestionTasks)
			protected.PUT("/ingestion/tasks", r.handler.StopIngestionTasks)
			protected.GET("/ingestion/tasks", r.handler.ListIngestionTasks)

			RegisterEERouter(protected, r)
		}
	}

	// Handle undefined routes
	engine.NoRoute(r.handler.HandleNoRoute)
}
