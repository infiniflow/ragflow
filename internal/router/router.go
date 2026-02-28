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

	"ragflow/internal/handler"
)

// Router router
type Router struct {
	userHandler          *handler.UserHandler
	tenantHandler        *handler.TenantHandler
	documentHandler      *handler.DocumentHandler
	systemHandler        *handler.SystemHandler
	knowledgebaseHandler *handler.KnowledgebaseHandler
	chunkHandler         *handler.ChunkHandler
	llmHandler           *handler.LLMHandler
	chatHandler          *handler.ChatHandler
	chatSessionHandler   *handler.ChatSessionHandler
	connectorHandler     *handler.ConnectorHandler
}

// NewRouter create router
func NewRouter(
	userHandler *handler.UserHandler,
	tenantHandler *handler.TenantHandler,
	documentHandler *handler.DocumentHandler,
	systemHandler *handler.SystemHandler,
	knowledgebaseHandler *handler.KnowledgebaseHandler,
	chunkHandler *handler.ChunkHandler,
	llmHandler *handler.LLMHandler,
	chatHandler *handler.ChatHandler,
	chatSessionHandler *handler.ChatSessionHandler,
	connectorHandler *handler.ConnectorHandler,
) *Router {
	return &Router{
		userHandler:          userHandler,
		tenantHandler:        tenantHandler,
		documentHandler:      documentHandler,
		systemHandler:        systemHandler,
		knowledgebaseHandler: knowledgebaseHandler,
		chunkHandler:         chunkHandler,
		llmHandler:           llmHandler,
		chatHandler:          chatHandler,
		chatSessionHandler:   chatSessionHandler,
		connectorHandler:     connectorHandler,
	}
}

// Setup setup routes
func (r *Router) Setup(engine *gin.Engine) {
	// Health check
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// System endpoints
	engine.GET("/v1/system/ping", r.systemHandler.Ping)
	engine.GET("/v1/system/config", r.systemHandler.GetConfig)
	engine.GET("/v1/system/configs", r.systemHandler.GetConfigs)
	engine.GET("/v1/system/version", r.systemHandler.GetVersion)

	// User login by email endpoint
	engine.POST("/v1/user/login", r.userHandler.LoginByEmail)
	// User login channels endpoint
	engine.GET("/v1/user/login/channels", r.userHandler.GetLoginChannels)
	// User logout endpoint
	engine.POST("/v1/user/logout", r.userHandler.Logout)
	// User info endpoint
	engine.GET("/v1/user/info", r.userHandler.Info)
	// User tenant info endpoint
	engine.GET("/v1/user/tenant_info", r.tenantHandler.TenantInfo)
	// Tenant list endpoint
	engine.GET("/v1/tenant/list", r.tenantHandler.TenantList)
	// User settings endpoint
	engine.POST("/v1/user/setting", r.userHandler.Setting)
	// User change password endpoint
	engine.POST("/v1/user/setting/password", r.userHandler.ChangePassword)

	// API v1 route group
	v1 := engine.Group("/api/v1")
	{
		// User routes
		users := v1.Group("/users")
		{
			users.POST("/register", r.userHandler.Register)
			users.POST("/login", r.userHandler.Login)
			users.GET("", r.userHandler.ListUsers)
			users.GET("/:id", r.userHandler.GetUserByID)
		}

		// Document routes
		documents := v1.Group("/documents")
		{
			documents.POST("", r.documentHandler.CreateDocument)
			documents.GET("", r.documentHandler.ListDocuments)
			documents.GET("/:id", r.documentHandler.GetDocumentByID)
			documents.PUT("/:id", r.documentHandler.UpdateDocument)
			documents.DELETE("/:id", r.documentHandler.DeleteDocument)
		}

		// Author routes
		authors := v1.Group("/authors")
		{
			authors.GET("/:author_id/documents", r.documentHandler.GetDocumentsByAuthorID)
		}

		// Knowledge base routes
		kb := engine.Group("/v1/kb")
		{
			kb.POST("/list", r.knowledgebaseHandler.ListKbs)
		}

		// Chunk routes
		chunk := engine.Group("/v1/chunk")
		{
			chunk.POST("/retrieval_test", r.chunkHandler.RetrievalTest)
		}

		// LLM routes
		llm := engine.Group("/v1/llm")
		{
			llm.GET("/my_llms", r.llmHandler.GetMyLLMs)
			llm.GET("/factories", r.llmHandler.Factories)
		}

		// Chat routes
		chat := engine.Group("/v1/dialog")
		{
			chat.GET("/list", r.chatHandler.ListChats)
			chat.POST("/next", r.chatHandler.ListChatsNext)
			chat.POST("/set", r.chatHandler.SetDialog)
			chat.POST("/rm", r.chatHandler.RemoveChats)
		}

		// Chat session (conversation) routes
		session := engine.Group("/v1/conversation")
		{
			session.POST("/set", r.chatSessionHandler.SetChatSession)
			session.POST("/rm", r.chatSessionHandler.RemoveChatSessions)
			session.GET("/list", r.chatSessionHandler.ListChatSessions)
			session.POST("/completion", r.chatSessionHandler.Completion)
		}

		// Connector routes
		connector := engine.Group("/v1/connector")
		{
			connector.GET("/list", r.connectorHandler.ListConnectors)
		}
	}

	// Handle undefined routes
	engine.NoRoute(handler.HandleNoRoute)
}
