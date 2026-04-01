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
	datasetsHandler      *handler.DatasetsHandler
	systemHandler        *handler.SystemHandler
	knowledgebaseHandler *handler.KnowledgebaseHandler
	chunkHandler         *handler.ChunkHandler
	llmHandler           *handler.LLMHandler
	chatHandler          *handler.ChatHandler
	chatSessionHandler   *handler.ChatSessionHandler
	connectorHandler     *handler.ConnectorHandler
	searchHandler        *handler.SearchHandler
	fileHandler          *handler.FileHandler
	memoryHandler        *handler.MemoryHandler
	skillSearchHandler   *handler.SkillSearchHandler
	providerHandler      *handler.ProviderHandler
}

// NewRouter create router
func NewRouter(
	authHandler *handler.AuthHandler,
	userHandler *handler.UserHandler,
	tenantHandler *handler.TenantHandler,
	documentHandler *handler.DocumentHandler,
	datasetsHandler *handler.DatasetsHandler,
	systemHandler *handler.SystemHandler,
	knowledgebaseHandler *handler.KnowledgebaseHandler,
	chunkHandler *handler.ChunkHandler,
	llmHandler *handler.LLMHandler,
	chatHandler *handler.ChatHandler,
	chatSessionHandler *handler.ChatSessionHandler,
	connectorHandler *handler.ConnectorHandler,
	searchHandler *handler.SearchHandler,
	fileHandler *handler.FileHandler,
	memoryHandler *handler.MemoryHandler,
	skillSearchHandler *handler.SkillSearchHandler,
	providerHandler *handler.ProviderHandler,
) *Router {
	return &Router{
		authHandler:          authHandler,
		userHandler:          userHandler,
		tenantHandler:        tenantHandler,
		documentHandler:      documentHandler,
		datasetsHandler:      datasetsHandler,
		systemHandler:        systemHandler,
		knowledgebaseHandler: knowledgebaseHandler,
		chunkHandler:         chunkHandler,
		llmHandler:           llmHandler,
		chatHandler:          chatHandler,
		chatSessionHandler:   chatSessionHandler,
		connectorHandler:     connectorHandler,
		searchHandler:        searchHandler,
		fileHandler:          fileHandler,
		memoryHandler:        memoryHandler,
		skillSearchHandler:   skillSearchHandler,
		providerHandler:      providerHandler,
	}
}

// Setup setup routes
func (r *Router) Setup(engine *gin.Engine) {
	// Health check
	engine.GET("/health", r.systemHandler.Health)

	// System endpoints
	engine.GET("/v1/system/ping", r.systemHandler.Ping)
	engine.GET("/v1/system/config", r.systemHandler.GetConfig)
	engine.GET("/v1/system/configs", r.systemHandler.GetConfigs)
	engine.GET("/v1/system/version", r.systemHandler.GetVersion)
	engine.GET("/v1/system/log_level", r.systemHandler.GetLogLevel)
	engine.PUT("/v1/system/log_level", r.systemHandler.SetLogLevel)
	engine.POST("/v1/user/register", r.userHandler.Register)
	// User login channels endpoint
	engine.GET("/v1/user/login/channels", r.userHandler.GetLoginChannels)

	// User login by email endpoint
	engine.POST("/v1/user/login", r.userHandler.LoginByEmail)

	// User logout endpoint
	engine.GET("/v1/user/logout", r.userHandler.Logout)

	// Protected routes
	authorized := engine.Group("")
	authorized.Use(r.authHandler.AuthMiddleware())
	{
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

		// System token endpoints (requires authentication)
		authorized.GET("/v1/system/token_list", r.systemHandler.ListTokens)
		authorized.POST("/v1/system/new_token", r.systemHandler.CreateToken)
		authorized.DELETE("/v1/system/token/:token", r.systemHandler.DeleteToken)

		// API v1 route group
		v1 := authorized.Group("/api/v1")
		{
			// User routes
			//users := v1.Group("/users")
			//{
			//	users.POST("/register", r.userHandler.Register)
			//	users.POST("/login", r.userHandler.Login)
			//	users.GET("", r.userHandler.ListUsers)
			//	users.GET("/:id", r.userHandler.GetUserByID)
			//}

			apiTokens := v1.Group("/tokens")
			{
				apiTokens.POST("", r.systemHandler.CreateToken)
				apiTokens.GET("", r.systemHandler.ListTokens)
				apiTokens.DELETE("/:token", r.systemHandler.DeleteToken)
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

			// RESTful dataset routes
			datasets := v1.Group("/datasets")
			{
				datasets.GET("", r.datasetsHandler.ListDatasets)
				datasets.POST("", r.datasetsHandler.CreateDataset)
				datasets.DELETE("", r.datasetsHandler.DeleteDatasets)
			}

			// Author routes
			authors := v1.Group("/authors")
			{
				authors.GET("/:author_id/documents", r.documentHandler.GetDocumentsByAuthorID)
			}

			// Memory routes
			memory := v1.Group("/memories")
			{
				memory.POST("", r.memoryHandler.CreateMemory)
				memory.PUT("/:memory_id", r.memoryHandler.UpdateMemory)
				memory.DELETE("/:memory_id", r.memoryHandler.DeleteMemory)
				memory.GET("", r.memoryHandler.ListMemories)
				memory.GET("/:memory_id/config", r.memoryHandler.GetMemoryConfig)
				memory.GET("/:memory_id", r.memoryHandler.GetMemoryMessages)
			}

			// TODO: Message routes - Implementation pending - depends on CanvasService, TaskService and embedding engine
			// message := v1.Group("/messages")
			// {
			// 	message.POST("", r.memoryHandler.AddMessage)
			// 	message.DELETE("/:memory_id/:message_id", r.memoryHandler.ForgetMessage)
			// 	message.PUT("/:memory_id/:message_id", r.memoryHandler.UpdateMessage)
			// 	message.GET("/search", r.memoryHandler.SearchMessage)
			// 	message.GET("", r.memoryHandler.GetMessages)
			// 	message.GET("/:memory_id/:message_id/content", r.memoryHandler.GetMessageContent)
			// }

			// Skill search routes
			skills := v1.Group("/skills")
			{
				skills.GET("/config", r.skillSearchHandler.GetConfig)
				skills.POST("/config", r.skillSearchHandler.UpdateConfig)
				skills.POST("/search", r.skillSearchHandler.Search)
				skills.POST("/index", r.skillSearchHandler.IndexSkills)
				skills.DELETE("/index/:skill_id", r.skillSearchHandler.DeleteSkillIndex)
				skills.POST("/reindex", r.skillSearchHandler.Reindex)
			}

			// provider pool route group
			provider := v1.Group("/providers")
			{
				provider.GET("/", r.providerHandler.ListProviders)
				provider.GET("/:provider_name", r.providerHandler.ShowProvider)
				provider.GET("/:provider_name/models", r.providerHandler.ListModels)
				provider.GET("/:provider_name/models/:model_name", r.providerHandler.ShowModel)
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
			kb.POST("/index", r.knowledgebaseHandler.CreateIndex)
			kb.DELETE("/index", r.knowledgebaseHandler.DeleteIndex)
			kb.POST("/insert_from_file", r.knowledgebaseHandler.InsertDatasetFromFile)

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

		// Tenant routes (per-tenant resources)
		tenant := authorized.Group("/v1/tenant")
		{
			tenant.POST("/doc_meta_index", r.tenantHandler.CreateDocMetaIndex)
			tenant.DELETE("/doc_meta_index", r.tenantHandler.DeleteDocMetaIndex)
			tenant.POST("/insert_metadata_from_file", r.tenantHandler.InsertMetadataFromFile)
		}

		// Document routes
		doc := authorized.Group("/v1/document")
		{
			doc.POST("/list", r.documentHandler.ListDocuments)
			doc.POST("/metadata/summary", r.documentHandler.MetadataSummary)
		}

		// Chunk routes
		chunk := authorized.Group("/v1/chunk")
		{
			chunk.POST("/retrieval_test", r.chunkHandler.RetrievalTest)
			chunk.GET("/get", r.chunkHandler.Get)
			chunk.POST("/list", r.chunkHandler.List)
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
