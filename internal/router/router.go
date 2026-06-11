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
	mcpHandler           *handler.MCPHandler
	skillSearchHandler   *handler.SkillSearchHandler
	providerHandler      *handler.ProviderHandler
	agentHandler         *handler.AgentHandler
	searchBotHandler     *handler.SearchBotHandler
	difyRetrievalHandler *handler.DifyRetrievalHandler
	pluginHandler        *handler.PluginHandler
	modelHandler         *handler.ModelHandler
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
	mcpHandler *handler.MCPHandler,
	skillSearchHandler *handler.SkillSearchHandler,
	providerHandler *handler.ProviderHandler,
	agentHandler *handler.AgentHandler,
	searchBotHandler *handler.SearchBotHandler,
	difyRetrievalHandler *handler.DifyRetrievalHandler,
	pluginHandler *handler.PluginHandler,
	modelHandler *handler.ModelHandler,
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
		mcpHandler:           mcpHandler,
		skillSearchHandler:   skillSearchHandler,
		providerHandler:      providerHandler,
		agentHandler:         agentHandler,
		searchBotHandler:     searchBotHandler,
		difyRetrievalHandler: difyRetrievalHandler,
		pluginHandler:        pluginHandler,
		modelHandler:         modelHandler,
	}
}

// Setup setup routes
func (r *Router) Setup(engine *gin.Engine) {
	// Mark all responses from Go with a header for debugging.
	engine.Use(func(c *gin.Context) {
		c.Header("X-API-Source", "go")
		c.Next()
	})

	// Log all HTTP requests.
	engine.Use(gin.Logger())

	// Health check
	engine.GET("/health", r.systemHandler.Health)

	// System endpoints
	engine.GET("/v1/system/configs", r.systemHandler.GetConfigs)
	//engine.POST("/v1/user/register", r.userHandler.Register)

	// User logout endpoint
	engine.GET("/v1/user/logout", r.userHandler.Logout)

	// OAuth callbacks are invoked by third-party providers and cannot rely on
	// the RAGFlow auth middleware.
	engine.GET("/connectors/gmail/oauth/web/callback", r.connectorHandler.GmailWebOAuthCallback)
	engine.GET("/connectors/google-drive/oauth/web/callback", r.connectorHandler.GoogleDriveWebOAuthCallback)

	apiNoAuth := engine.Group("/api/v1")
	{
		apiNoAuth.GET("/system/ping", r.systemHandler.Ping)
		apiNoAuth.GET("/system/config", r.systemHandler.GetConfig)
		apiNoAuth.GET("/system/version", r.systemHandler.GetVersion)
		apiNoAuth.GET("/system/healthz", r.systemHandler.Healthz)

		// User login channels endpoint
		apiNoAuth.GET("/auth/login/channels", r.userHandler.GetLoginChannels)

		// User login by email endpoint
		apiNoAuth.POST("/auth/login", r.userHandler.LoginByEmail)

		// OAuth / OIDC login routes. The static "channels" segment is
		// registered before the wildcard, so gin's tree resolves
		// /auth/login/channels to GetLoginChannels and other values to
		// OAuthLogin without conflict.
		apiNoAuth.GET("/auth/login/:channel", r.userHandler.OAuthLogin)
		apiNoAuth.GET("/auth/oauth/:channel/callback", r.userHandler.OAuthCallback)

		// Register
		apiNoAuth.POST("/users", r.userHandler.Register)

		// Document images are embedded directly in pages and match Python's public route.
		apiNoAuth.GET("/documents/images/:image_id", r.documentHandler.GetDocumentImage)

		// Google redirects here after Gmail / Google Drive web OAuth completes.
		apiNoAuth.GET("/connectors/gmail/oauth/web/callback", r.connectorHandler.GmailWebOAuthCallback)
		apiNoAuth.GET("/connectors/google-drive/oauth/web/callback", r.connectorHandler.GoogleDriveWebOAuthCallback)
		// Forgot-password flow (fixes #15282).
		// Routes are intentionally registered before any auth middleware:
		// a user who has forgotten their password is, by definition,
		// unauthenticated.
		apiNoAuth.POST("/auth/password/forgot/captcha", r.userHandler.ForgotCaptcha)
		apiNoAuth.POST("/auth/password/forgot/otp", r.userHandler.ForgotSendOTP)
		apiNoAuth.POST("/auth/password/forgot/otp/verify", r.userHandler.ForgotVerifyOTP)
		apiNoAuth.POST("/auth/password/reset", r.userHandler.ForgotResetPassword)
	}

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

		// API v1 route group
		v1 := authorized.Group("/api/v1")
		{
			// Auth routes
			auth := v1.Group("/auth")
			{
				// User logout endpoint
				auth.POST("/logout", r.userHandler.Logout)
			}

			// Users routes
			users := v1.Group("/users")
			{
				users.GET("/me", r.userHandler.Info)
				// User settings endpoint
				users.PATCH("/me", r.userHandler.Setting)
				// User tenant info endpoint
				users.GET("/me/models", r.tenantHandler.TenantInfo)
				// User set tenant info endpoint
				users.PATCH("/me/models", r.userHandler.SetTenantInfo)
			}

			tenants := v1.Group("/tenants")
			{
				tenants.GET("", r.tenantHandler.TenantList)
				tenants.PATCH("/:tenant_id", r.tenantHandler.AcceptTenantInvite)
				tenants.GET("/:tenant_id/users", r.tenantHandler.ListTenantMembers)
				tenants.POST("/:tenant_id/users", r.tenantHandler.AddTenantMember)
				tenants.DELETE("/:tenant_id/users", r.tenantHandler.RemoveTenantMember)
			}

			v1.GET("/tenant/list", r.tenantHandler.TenantList)

			// Document routes
			documents := v1.Group("/documents")
			{
				documents.POST("", r.documentHandler.CreateDocument)
				documents.GET("", r.documentHandler.ListDocuments)
				documents.GET("/artifact/:filename", r.documentHandler.GetDocumentArtifact)
				documents.GET("/:id/preview", r.documentHandler.GetDocumentPreview)
				documents.GET("/:id", r.documentHandler.GetDocumentByID)
				documents.PUT("/:id", r.documentHandler.UpdateDocument)
				documents.DELETE("/:id", r.documentHandler.DeleteDocument)
			}

			// Chat routes
			chats := v1.Group("/chats")
			{
				chats.GET("", r.chatHandler.ListChats)
				chats.GET("/:chat_id", r.chatHandler.GetChat)
				chats.GET("/:chat_id/sessions", r.chatSessionHandler.ListChatSessions)
			}

			// Searchbot routes
			v1.POST("/searchbots/related_questions", r.searchBotHandler.Handle)
			v1.POST("/searchbots/retrieval_test", r.searchBotHandler.RetrievalTest)
			v1.POST("/searchbots/ask", r.searchBotHandler.Ask)

			// Dataset routes
			datasets := v1.Group("/datasets")
			{
				datasets.GET("", r.datasetsHandler.ListDatasets)
				datasets.GET("/:dataset_id", r.datasetsHandler.GetDataset)
				datasets.GET("/:dataset_id/graph", r.datasetsHandler.GetKnowledgeGraph)
				datasets.DELETE("/:dataset_id/tags", r.datasetsHandler.RemoveTags)
				datasets.DELETE("/:dataset_id/graph", r.datasetsHandler.DeleteKnowledgeGraph)
				datasets.POST("", r.datasetsHandler.CreateDataset)
				datasets.DELETE("", r.datasetsHandler.DeleteDatasets)
				datasets.POST("/search", r.datasetsHandler.SearchDatasets)
				datasets.GET("/metadata/flattened", r.datasetsHandler.ListMetadataFlattened)
				datasets.GET("/:dataset_id/metadata/summary", r.documentHandler.MetadataSummaryByDataset)

				// Dataset ingestion logs
				datasets.GET("/:dataset_id/ingestions/summary", r.datasetsHandler.GetIngestionSummary)
				datasets.GET("/:dataset_id/ingestions", r.datasetsHandler.ListIngestionLogs)
				datasets.GET("/:dataset_id/ingestions/:log_id", r.datasetsHandler.GetIngestionLog)

				// Metadata Config
				datasets.GET("/:dataset_id/metadata/config", r.datasetsHandler.GetMetadataConfig)
				datasets.PUT("/:dataset_id/metadata/config", r.datasetsHandler.UpdateMetadataConfig)

				// Dataset documents
				datasets.GET("/:dataset_id/documents", r.documentHandler.ListDocuments)
				datasets.GET("/:dataset_id/documents/:document_id", r.documentHandler.DownloadDocument)
				datasets.DELETE("/:dataset_id/documents", r.documentHandler.DeleteDocuments)

				// Dataset document chunk
				datasets.GET("/:dataset_id/documents/:document_id/chunks/:chunk_id", r.chunkHandler.Get)
				datasets.POST("/:dataset_id/documents/parse", r.documentHandler.ParseDocuments)
				datasets.POST("/:dataset_id/documents/stop", r.documentHandler.StopParseDocuments)
				datasets.DELETE("/:dataset_id/documents/:document_id/chunks", r.chunkHandler.RemoveChunks)
				datasets.PUT("/:dataset_id/documents/:document_id/metadata/config", r.datasetsHandler.UpdateDocumentMetadataConfig)
			}

			// Search routes
			searches := v1.Group("/searches")
			{
				searches.GET("", r.searchHandler.ListSearches)
				searches.POST("", r.searchHandler.CreateSearch)
				searches.GET("/:search_id", r.searchHandler.GetSearch)
				searches.PUT("/:search_id", r.searchHandler.UpdateSearch)
				searches.DELETE("/:search_id", r.searchHandler.DeleteSearch)
			}

			file := v1.Group("/files")
			{
				file.POST("", r.fileHandler.UploadFile)
				file.GET("", r.fileHandler.ListFiles)
				file.DELETE("", r.fileHandler.DeleteFiles)
				file.POST("/move", r.fileHandler.MoveFiles)
				file.POST("/link-to-datasets", r.fileHandler.LinkToDatasets)
				file.GET("/:id/ancestors", r.fileHandler.GetFileAncestors)
				file.GET("/:id/parent", r.fileHandler.GetParentFolder)
				file.GET("/:id", r.fileHandler.Download)
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

			// Message routes
			message := v1.Group("/messages")
			{
				message.DELETE("/:memory_message", r.memoryHandler.ForgetMessage)
			}

			// Skill search routes
			skills := v1.Group("/skills")
			{
				// Skill Space management
				skills.GET("/spaces", r.skillSearchHandler.ListSpaces)
				skills.POST("/spaces", r.skillSearchHandler.CreateSpace)
				skills.GET("/spaces/:space_id", r.skillSearchHandler.GetSpace)
				skills.PUT("/spaces/:space_id", r.skillSearchHandler.UpdateSpace)
				skills.DELETE("/spaces/:space_id", r.skillSearchHandler.DeleteSpace)
				skills.GET("/space/by-folder", r.skillSearchHandler.GetSpaceByFolder)

				// Skill search config
				skills.GET("/config", r.skillSearchHandler.GetConfig)
				skills.POST("/config", r.skillSearchHandler.UpdateConfig)

				// Skill search and indexing
				skills.POST("/search", r.skillSearchHandler.Search)
				skills.POST("/index", r.skillSearchHandler.IndexSkills)
				skills.DELETE("/index", r.skillSearchHandler.DeleteSkillIndex)
				skills.POST("/reindex", r.skillSearchHandler.Reindex)
			}

			// provider pool route group
			provider := v1.Group("/providers")
			{
				provider.GET("/", r.providerHandler.ListProviders)
				provider.PUT("/", r.providerHandler.AddProvider)
				provider.GET("/:provider_name", r.providerHandler.ShowProvider)
				provider.DELETE("/:provider_name", r.providerHandler.DeleteProvider)
				provider.GET("/:provider_name/models", r.providerHandler.ListModels)
				provider.GET("/:provider_name/models/:model_name", r.providerHandler.ShowModel)
				provider.POST("/:provider_name/instances", r.providerHandler.CreateProviderInstance)
				provider.GET("/:provider_name/instances", r.providerHandler.ListProviderInstances)
				provider.GET("/:provider_name/instances/:instance_name", r.providerHandler.ShowProviderInstance)
				provider.GET("/:provider_name/instances/:instance_name/balance", r.providerHandler.ShowInstanceBalance)
				provider.GET("/:provider_name/instances/:instance_name/connection", r.providerHandler.CheckInstanceConnection)
				provider.GET("/:provider_name/connection", r.providerHandler.CheckConnection)
				provider.GET("/:provider_name/instances/:instance_name/tasks", r.providerHandler.ListTasks)
				provider.GET("/:provider_name/instances/:instance_name/tasks/:task_id", r.providerHandler.ShowTask)
				provider.PUT("/:provider_name/instances/:instance_name", r.providerHandler.AlterProviderInstance)
				provider.DELETE("/:provider_name/instances", r.providerHandler.DropProviderInstance)
				provider.GET("/:provider_name/instances/:instance_name/models", r.providerHandler.ListInstanceModels)
				provider.PATCH("/:provider_name/instances/:instance_name/models/*model_name", r.providerHandler.EnableOrDisableModel)
				provider.POST("/:provider_name/instances/:instance_name/models", r.providerHandler.AddModel)
				provider.DELETE("/:provider_name/instances/:instance_name/models", r.providerHandler.DropInstanceModels)
				v1.POST("/chat/completions", r.providerHandler.ChatToModel)
				v1.POST("/embeddings", r.providerHandler.EmbedText)
				v1.POST("/rerank", r.providerHandler.RerankDocument)
				v1.POST("/audio/transcriptions", r.providerHandler.TranscribeAudio)
				v1.POST("/audio/speech", r.providerHandler.AudioSpeech)
				v1.POST("/file/ocr", r.providerHandler.OCRFile)
				v1.POST("/file/parse", r.providerHandler.ParseFile)
			}

			model := v1.Group("/models")
			{
				model.GET("/", r.tenantHandler.GetModels)
				model.PATCH("/", r.tenantHandler.SetModels)
			}

			allModels := v1.Group("/all-models")
			{
				allModels.GET("", r.modelHandler.ListAllModels)
			}

			// Agent routes
			agents := v1.Group("/agents")
			{
				agents.GET("", r.agentHandler.ListAgents)
				agents.GET("/prompts", r.agentHandler.GetPrompts)
				agents.GET("/templates", r.agentHandler.ListTemplates)
				agents.GET("/download", r.agentHandler.DownloadAgentFile)
				agents.POST("/test_db_connection", r.agentHandler.TestDBConnection)
				agents.GET("/:agent_id/versions", r.agentHandler.ListAgentVersions)
				agents.GET("/:agent_id/versions/:version_id", r.agentHandler.GetAgentVersion)
				agents.POST("/:agent_id/upload", r.agentHandler.UploadAgentFile)
				agents.PUT("/:agent_id/tags", r.agentHandler.UpdateAgentTags)
				agents.GET("/:agent_id/sessions", r.agentHandler.ListAgentSessions)
				agents.GET("/:agent_id/sessions/:session_id", r.agentHandler.GetAgentSession)
				agents.DELETE("/:agent_id/sessions/:session_id", r.agentHandler.DeleteAgentSessionItem)
				agents.DELETE("/:agent_id/sessions", r.agentHandler.DeleteAgentSessions)
			}

			// Plugin routes
			plugin := v1.Group("/plugin")
			{
				plugin.GET("/tools", r.pluginHandler.ListLLMTools)
			}

			connector := v1.Group("/connectors")
			{
				connector.GET("/", r.connectorHandler.ListConnectors)
				connector.POST("/", r.connectorHandler.CreateConnector)
				connector.POST("/google/oauth/web/start", r.connectorHandler.StartGoogleWebOAuth)
				connector.POST("/google/oauth/web/result", r.connectorHandler.PollGoogleWebOAuthResult)
				connector.GET("/:connector_id", r.connectorHandler.GetConnector)
				connector.GET("/:connector_id/logs", r.connectorHandler.ListLogs)
				connector.DELETE("/:connector_id", r.connectorHandler.DeleteConnector)
				connector.POST("/:connector_id/rebuild", r.connectorHandler.RebuildConnector)
				connector.POST("/:connector_id/test", r.connectorHandler.TestConnector)
			}

			// MCP server routes. Per-server CRUD ships via separate PRs that
			// share the same handler/service: GET list (#15253), GET by id
			// (#15254), POST create (#15260, merged), PUT (#15261), DELETE
			// (#15262, merged). This PR adds only the non-overlapping
			// endpoints: import and test.
			mcp := v1.Group("/mcp")
			{
				mcp.POST("/servers", r.mcpHandler.CreateMCPServer)
				mcp.GET("/servers", r.mcpHandler.ListMCPServers)
				mcp.PUT("/servers/:mcp_id", r.mcpHandler.UpdateMCPServer)
				mcp.DELETE("/servers/:mcp_id", r.mcpHandler.DeleteMCPServer)
				mcp.POST("/servers/import", r.mcpHandler.ImportMCPServers)
				mcp.POST("/servers/:mcp_id/test", r.mcpHandler.TestMCPServer)
			}

			system := v1.Group("/system")
			{
				system.GET("/configs", r.systemHandler.GetConfigs)
				system.GET("/status", r.systemHandler.GetStatus)
				system.GET("/stats", r.systemHandler.GetStats)

				config := system.Group("/config")
				{
					config.GET("/log", r.systemHandler.GetLogLevel)
					config.PUT("/log", r.systemHandler.SetLogLevel)
				}

				//log := system.Group("/log")
				//{
				//	// /api/v1/system/log GET
				//	log.GET("", r.systemHandler.GetLogLevel)
				//	// /api/v1/system/log PUT
				//	log.PUT("", r.systemHandler.SetLogLevel)
				//}

				tokens := system.Group("/tokens")
				{
					// list tokens /api/v1/system/tokens GET
					tokens.GET("", r.systemHandler.ListTokens)
					// create token /api/v1/system/tokens POST
					tokens.POST("", r.systemHandler.CreateToken)
					// delete token /api/v1/system/tokens/:token DELETE
					tokens.DELETE("/:token", r.systemHandler.DeleteToken)
				}
			}
		}

		// Knowledge base routes
		kb := v1.Group("/kb")
		{
			kb.POST("/update", r.knowledgebaseHandler.UpdateKB)
			kb.POST("/update_metadata_setting", r.knowledgebaseHandler.UpdateMetadataSetting)
			kb.GET("/detail", r.knowledgebaseHandler.GetDetail)
			kb.GET("/tags", r.knowledgebaseHandler.ListTagsFromKbs)
			kb.GET("/get_meta", r.knowledgebaseHandler.GetMeta)
			kb.GET("/basic_info", r.knowledgebaseHandler.GetBasicInfo)

			// KB ID specific routes
			kbByID := kb.Group("/:kb_id")
			{
				kbByID.GET("/tags", r.knowledgebaseHandler.ListTags)
				kbByID.POST("/rename_tag", r.knowledgebaseHandler.RenameTag)
				kbByID.GET("/knowledge_graph", r.knowledgebaseHandler.KnowledgeGraph)
				kbByID.DELETE("/knowledge_graph", r.knowledgebaseHandler.DeleteKnowledgeGraph)
			}
		}

		// Tenant routes (per-tenant resources)
		tenant := v1.Group("/tenant")
		{
			tenant.POST("/chunk_store", r.tenantHandler.CreateChunkStore)                     // Internal API only for GO
			tenant.DELETE("/chunk_store", r.tenantHandler.DeleteChunkStore)                   // Internal API only for GO
			tenant.POST("/metadata_store", r.tenantHandler.CreateMetadataStore)               // Internal API only for GO
			tenant.DELETE("/metadata_store", r.tenantHandler.DeleteMetadataStore)             // Internal API only for GO
			tenant.POST("/insert_chunks_from_file", r.tenantHandler.InsertChunksFromFile)     // Internal API only for GO
			tenant.POST("/insert_metadata_from_file", r.tenantHandler.InsertMetadataFromFile) // Internal API only for GO
		}

		// Document routes
		doc := v1.Group("/document")
		{
			doc.POST("/list", r.documentHandler.ListDocuments)
			doc.POST("/metadata/summary", r.documentHandler.MetadataSummary)
			doc.POST("/set_meta", r.documentHandler.SetMeta)
			doc.POST("/delete_meta", r.documentHandler.DeleteMeta) // Internal API only for GO
		}

		v1.GET("/thumbnails", r.documentHandler.GetThumbnail)

		// Chunk routes
		chunk := v1.Group("/chunk")
		{
			chunk.POST("/list", r.chunkHandler.List)
			chunk.POST("/update", r.chunkHandler.UpdateChunk) // Internal API only for GO
		}

		// Chat routes
		chat := authorized.Group("/v1/dialog")
		{
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
			connector.GET("/:connector_id", r.connectorHandler.GetConnector)
			connector.POST("/:connector_id/rebuild", r.connectorHandler.RebuildConnector)
		}

		// File routes
		file := authorized.Group("/v1/file")
		{
			file.GET("/root_folder", r.fileHandler.GetRootFolder)
			file.GET("/parent_folder", r.fileHandler.GetParentFolder)
			file.GET("/all_parent_folder", r.fileHandler.GetAllParentFolders)
		}

	}

	// Dify retrieval routes
	dify := authorized.Group("/api/v1/dify")
	{
		dify.POST("/retrieval", r.difyRetrievalHandler.Retrieval)
		dify.GET("/retrieval", r.difyRetrievalHandler.Retrieval)
	}
	apiNoAuth.GET("/dify/retrieval/health", r.difyRetrievalHandler.HealthCheck)

	// Handle undefined routes
	engine.NoRoute(handler.HandleNoRoute)
}
