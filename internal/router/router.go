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
	authHandler          *handler.AuthHandler
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
	searchHandler        *handler.SearchHandler
	fileHandler          *handler.FileHandler
}

// NewRouter create router
func NewRouter(
	authHandler *handler.AuthHandler,
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
	searchHandler *handler.SearchHandler,
	fileHandler *handler.FileHandler,
) *Router {
	return &Router{
		authHandler:          authHandler,
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
		searchHandler:        searchHandler,
		fileHandler:          fileHandler,
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
	engine.POST("/v1/user/register", r.userHandler.Register)
	// User login channels endpoint
	engine.GET("/v1/user/login/channels", r.userHandler.GetLoginChannels)

	// User login by email endpoint
	engine.POST("/v1/user/login", r.userHandler.LoginByEmail)

	// Protected routes
	authorized := engine.Group("")
	authorized.Use(r.authHandler.AuthMiddleware())
	{
		// User logout endpoint
		authorized.GET("/v1/user/logout", r.userHandler.Logout)
		// User info endpoint
		authorized.GET("/v1/user/info", r.userHandler.Info)
		// User tenant info endpoint
		authorized.GET("/v1/user/tenant_info", r.tenantHandler.TenantInfo)
		// Tenant list endpoint
		authorized.GET("/v1/tenant/list", r.tenantHandler.TenantList)
		// User settings endpoint
		authorized.POST("/v1/user/setting", r.userHandler.Setting)
		// User change password endpoint
		authorized.POST("/v1/user/setting/password", r.userHandler.ChangePassword)
		// User set tenant info endpoint
		authorized.POST("/v1/user/set_tenant_info", r.userHandler.SetTenantInfo)

		// API v1 route group
		v1 := authorized.Group("/api/v1")
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
		}

		// Knowledge base routes
		kb := authorized.Group("/v1/kb")
		{
			kb.POST("/create", r.knowledgebaseHandler.CreateKB)
			kb.POST("/update", r.knowledgebaseHandler.UpdateKB)
			kb.POST("/update_metadata_setting", r.knowledgebaseHandler.UpdateMetadataSetting)
			kb.GET("/detail", r.knowledgebaseHandler.GetDetail)
			kb.POST("/list", r.knowledgebaseHandler.ListKbs)
			kb.POST("/rm", r.knowledgebaseHandler.DeleteKB)
			kb.GET("/tags", r.knowledgebaseHandler.ListTagsFromKbs)
			kb.GET("/get_meta", r.knowledgebaseHandler.GetMeta)
			kb.GET("/basic_info", r.knowledgebaseHandler.GetBasicInfo)

			// KB ID specific routes
			kbByID := kb.Group("/:kb_id")
			{
				kbByID.GET("/tags", r.knowledgebaseHandler.ListTags)
				kbByID.POST("/rm_tags", r.knowledgebaseHandler.RemoveTags)
				kbByID.POST("/rename_tag", r.knowledgebaseHandler.RenameTag)
				kbByID.GET("/knowledge_graph", r.knowledgebaseHandler.KnowledgeGraph)
				kbByID.DELETE("/knowledge_graph", r.knowledgebaseHandler.DeleteKnowledgeGraph)
			}
		}

		// Chunk routes
		chunk := authorized.Group("/v1/chunk")
		{
			chunk.POST("/retrieval_test", r.chunkHandler.RetrievalTest)
		}

		// LLM routes
		llm := authorized.Group("/v1/llm")
		{
			llm.GET("/my_llms", r.llmHandler.GetMyLLMs)
			llm.GET("/factories", r.llmHandler.Factories)
			llm.GET("/list", r.llmHandler.ListApp)
			llm.POST("/set_api_key", r.llmHandler.SetAPIKey)
		}

		// Chat routes
		chat := authorized.Group("/v1/dialog")
		{
			chat.GET("/list", r.chatHandler.ListChats)
			chat.POST("/next", r.chatHandler.ListChatsNext)
			chat.POST("/set", r.chatHandler.SetDialog)
			chat.POST("/rm", r.chatHandler.RemoveChats)
		}

		// Chat session (conversation) routes
		session := authorized.Group("/v1/conversation")
		{
			session.POST("/set", r.chatSessionHandler.SetChatSession)
			session.POST("/rm", r.chatSessionHandler.RemoveChatSessions)
			session.GET("/list", r.chatSessionHandler.ListChatSessions)
			session.POST("/completion", r.chatSessionHandler.Completion)
		}

		// Connector routes
		connector := authorized.Group("/v1/connector")
		{
			connector.GET("/list", r.connectorHandler.ListConnectors)
		}

		// Search routes
		search := authorized.Group("/v1/search")
		{
			search.POST("/list", r.searchHandler.ListSearchApps)
		}

		// File routes
		file := authorized.Group("/v1/file")
		{
			file.GET("/list", r.fileHandler.ListFiles)
			file.GET("/root_folder", r.fileHandler.GetRootFolder)
			file.GET("/parent_folder", r.fileHandler.GetParentFolder)
			file.GET("/all_parent_folder", r.fileHandler.GetAllParentFolders)
		}
	}

	// Handle undefined routes
	engine.NoRoute(handler.HandleNoRoute)
}
