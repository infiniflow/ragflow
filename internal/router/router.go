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

	"ragflow/internal/common"
	"ragflow/internal/handler"
)

type Router struct {
	authHandler          *handler.AuthHandler
	userHandler          *handler.UserHandler
	tenantHandler        *handler.TenantHandler
	documentHandler      *handler.DocumentHandler
	datasetsHandler      *handler.DatasetsHandler
	systemHandler        *handler.SystemHandler
	chunkHandler         *handler.ChunkHandler
	llmHandler           *handler.LLMHandler
	chatHandler          *handler.ChatHandler
	chatChannelHandler   *handler.ChatChannelHandler
	langfuseHandler      *handler.LangfuseHandler
	openaiChatHandler    *handler.OpenAIChatHandler
	chatSessionHandler   *handler.ChatSessionHandler
	connectorHandler     *handler.ConnectorHandler
	searchHandler        *handler.SearchHandler
	fileHandler          *handler.FileHandler
	memoryHandler        *handler.MemoryHandler
	mcpHandler           *handler.MCPHandler
	mcpServerHandler     *handler.MCPServerHandler
	skillSearchHandler   *handler.SkillSearchHandler
	providerHandler      *handler.ProviderHandler
	agentHandler         *handler.AgentHandler
	searchBotHandler     *handler.SearchBotHandler
	difyRetrievalHandler *handler.DifyRetrievalHandler
	pluginHandler        *handler.PluginHandler
	modelHandler         *handler.ModelHandler
	fileCommitHandler    *handler.FileCommitHandler
	adminRuntimeHandler  *handler.AdminRuntimeHandler
	botHandler           *handler.BotHandler
	componentsHandler    *handler.ComponentsHandler
}

// NewRouter create router
func NewRouter(
	authHandler *handler.AuthHandler,
	userHandler *handler.UserHandler,
	tenantHandler *handler.TenantHandler,
	documentHandler *handler.DocumentHandler,
	datasetsHandler *handler.DatasetsHandler,
	systemHandler *handler.SystemHandler,
	chunkHandler *handler.ChunkHandler,
	llmHandler *handler.LLMHandler,
	chatHandler *handler.ChatHandler,
	chatChannelHandler *handler.ChatChannelHandler,
	langfuseHandler *handler.LangfuseHandler,
	chatSessionHandler *handler.ChatSessionHandler,
	connectorHandler *handler.ConnectorHandler,
	searchHandler *handler.SearchHandler,
	fileHandler *handler.FileHandler,
	memoryHandler *handler.MemoryHandler,
	mcpHandler *handler.MCPHandler,
	mcpServerHandler *handler.MCPServerHandler,
	skillSearchHandler *handler.SkillSearchHandler,
	providerHandler *handler.ProviderHandler,
	agentHandler *handler.AgentHandler,
	searchBotHandler *handler.SearchBotHandler,
	difyRetrievalHandler *handler.DifyRetrievalHandler,
	pluginHandler *handler.PluginHandler,
	modelHandler *handler.ModelHandler,
	fileCommitHandler *handler.FileCommitHandler,
	adminRuntimeHandler *handler.AdminRuntimeHandler,
	openaiChatHandler *handler.OpenAIChatHandler,
	botHandler *handler.BotHandler,
	componentsHandler *handler.ComponentsHandler,
) *Router {
	return &Router{
		authHandler:          authHandler,
		userHandler:          userHandler,
		tenantHandler:        tenantHandler,
		documentHandler:      documentHandler,
		datasetsHandler:      datasetsHandler,
		systemHandler:        systemHandler,
		chunkHandler:         chunkHandler,
		llmHandler:           llmHandler,
		chatHandler:          chatHandler,
		chatChannelHandler:   chatChannelHandler,
		langfuseHandler:      langfuseHandler,
		openaiChatHandler:    openaiChatHandler,
		chatSessionHandler:   chatSessionHandler,
		connectorHandler:     connectorHandler,
		searchHandler:        searchHandler,
		fileHandler:          fileHandler,
		memoryHandler:        memoryHandler,
		mcpHandler:           mcpHandler,
		mcpServerHandler:     mcpServerHandler,
		skillSearchHandler:   skillSearchHandler,
		providerHandler:      providerHandler,
		agentHandler:         agentHandler,
		searchBotHandler:     searchBotHandler,
		difyRetrievalHandler: difyRetrievalHandler,
		pluginHandler:        pluginHandler,
		modelHandler:         modelHandler,
		fileCommitHandler:    fileCommitHandler,
		adminRuntimeHandler:  adminRuntimeHandler,
		botHandler:           botHandler,
		componentsHandler:    componentsHandler,
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
	engine.Use(common.GinLogger())

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
	engine.GET("/connectors/box/oauth/web/callback", r.connectorHandler.BoxWebOAuthCallback)

	apiNoAuth := engine.Group("/api/v1")
	{
		apiNoAuth.GET("/system/ping", r.systemHandler.Ping)
		apiNoAuth.GET("/system/config", r.systemHandler.GetConfig)
		apiNoAuth.GET("/system/version", r.systemHandler.GetVersion)
		apiNoAuth.GET("/system/healthz", r.systemHandler.Healthz)

		// searchbots
		apiNoAuth.GET("/searchbots/detail", r.searchBotHandler.SearchbotDetail)

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

		// Google redirects here after Gmail / Google Drive web OAuth completes.
		apiNoAuth.GET("/connectors/gmail/oauth/web/callback", r.connectorHandler.GmailWebOAuthCallback)
		apiNoAuth.GET("/connectors/google-drive/oauth/web/callback", r.connectorHandler.GoogleDriveWebOAuthCallback)
		apiNoAuth.GET("/connectors/box/oauth/web/callback", r.connectorHandler.BoxWebOAuthCallback)
		// Forgot-password flow (fixes #15282).
		// Routes are intentionally registered before any auth middleware:
		// a user who has forgotten their password is, by definition,
		// unauthenticated.
		apiNoAuth.POST("/auth/password/forgot/captcha", r.userHandler.ForgotCaptcha)
		apiNoAuth.POST("/auth/password/forgot/otp", r.userHandler.ForgotSendOTP)
		apiNoAuth.POST("/auth/password/forgot/otp/verify", r.userHandler.ForgotVerifyOTP)
		apiNoAuth.POST("/auth/password/reset", r.userHandler.ForgotResetPassword)

		apiNoAuth.GET("/dify/retrieval/health", r.difyRetrievalHandler.HealthCheck)
	}

	// Beta-token routes. Mirrors python's
	// @login_required(auth_types=AUTH_BETA) on bot_api.py bot endpoints.
	apiBetaAuth := engine.Group("/api/v1")
	apiBetaAuth.Use(r.authHandler.BetaAuthMiddleware())
	{
		searchbotGroup := apiBetaAuth.Group("/searchbots")
		searchbotGroup.POST("/related_questions", r.searchBotHandler.Handle)
		searchbotGroup.POST("/retrieval_test", r.searchBotHandler.RetrievalTest)
		searchbotGroup.POST("/ask", r.searchBotHandler.Ask)
		searchbotGroup.POST("/mindmap", r.searchBotHandler.MindMap)

		if r.botHandler != nil {
			chatbotGroup := apiBetaAuth.Group("/chatbots")
			betaMW := r.authHandler.BetaAuthMiddleware()
			RegisterChatbotRoutes(chatbotGroup, betaMW, r.botHandler)
			agentbotGroup := apiBetaAuth.Group("/agentbots")
			RegisterAgentbotRoutes(agentbotGroup, betaMW, r.botHandler)
		}
		// Public bot endpoints (authenticated with an SDK beta token, not a session)
		apiBetaAuth.GET("/documents/:id/preview", r.documentHandler.GetDocumentPreview)
		apiBetaAuth.GET("/documents/images/:image_id", r.documentHandler.GetDocumentImage)
		apiBetaAuth.GET("/thumbnails", r.documentHandler.GetThumbnail)

		// MCP server endpoint — exposes RAGFlow capabilities as MCP tools.
		// Uses BetaAuthMiddleware to resolve the user from the
		// Authorization header.
		if r.mcpServerHandler != nil {
			apiBetaAuth.POST("/mcp", r.mcpServerHandler.HandleMCP)
		}
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

			// Tenant routes (per-tenant resources)
			tenant := v1.Group("/tenant")
			{
				tenant.GET("/list", r.tenantHandler.TenantList)
				tenant.POST("/chunk_store", r.tenantHandler.CreateChunkStore)                     // Internal API only for GO
				tenant.DELETE("/chunk_store", r.tenantHandler.DeleteChunkStore)                   // Internal API only for GO
				tenant.POST("/metadata_store", r.tenantHandler.CreateMetadataStore)               // Internal API only for GO
				tenant.DELETE("/metadata_store", r.tenantHandler.DeleteMetadataStore)             // Internal API only for GO
				tenant.POST("/insert_chunks_from_file", r.tenantHandler.InsertChunksFromFile)     // Internal API only for GO
				tenant.POST("/insert_metadata_from_file", r.tenantHandler.InsertMetadataFromFile) // Internal API only for GO
			}

			// Document routes
			documents := v1.Group("/documents")
			{
				documents.POST("", r.documentHandler.CreateDocument)
				documents.POST("/upload", r.documentHandler.UploadInfo)
				documents.GET("", r.documentHandler.ListDocuments)
				documents.GET("/artifact/:filename", r.documentHandler.GetDocumentArtifact)
				documents.GET("/:id", r.documentHandler.GetDocumentByID)
				documents.PUT("/:id", r.documentHandler.UpdateDocument)
				documents.DELETE("/:id", r.documentHandler.DeleteDocument)
				documents.POST("/ingest", r.documentHandler.Ingest)
			}

			// Chat routes
			chats := v1.Group("/chats")
			{
				chats.GET("", r.chatHandler.ListChats)
				chats.POST("", r.chatHandler.Create)
				chats.DELETE("", r.chatHandler.BulkDeleteChats)
				chats.DELETE("/:chat_id", r.chatHandler.DeleteChat)
				chats.GET("/:chat_id", r.chatHandler.GetChat)
				chats.PUT("/:chat_id", r.chatHandler.UpdateChat)
				chats.PATCH("/:chat_id", r.chatHandler.PatchChat)
				chats.GET("/:chat_id/sessions", r.chatSessionHandler.ListChatSessions)
				chats.POST("/:chat_id/sessions", r.chatSessionHandler.CreateSession)
				chats.DELETE("/:chat_id/sessions", r.chatSessionHandler.DeleteSessions)
				chats.GET("/:chat_id/sessions/:session_id", r.chatSessionHandler.GetSession)
				chats.PATCH("/:chat_id/sessions/:session_id", r.chatSessionHandler.UpdateSession)
				chats.DELETE("/:chat_id/sessions/:session_id/messages/:msg_id", r.chatSessionHandler.DeleteSessionMessage)
				chats.PUT("/:chat_id/sessions/:session_id/messages/:msg_id/feedback", r.chatSessionHandler.UpdateMessageFeedback)
			}

			chat := v1.Group("/chat")
			{
				chat.POST("/completions", r.chatSessionHandler.ChatCompletions)
				chat.POST("/mindmap", r.chatHandler.MindMap)
				chat.POST("/recommendation", r.chatHandler.Recommendation)
			}
			v1.POST("/openai/:chat_id/chat/completions", r.openaiChatHandler.OpenAIChatCompletions)

			// Dataset routes
			datasets := v1.Group("/datasets")
			{
				datasets.GET("", r.datasetsHandler.ListDatasets)
				datasets.GET("/tags/aggregation", r.datasetsHandler.AggregateTags)
				datasets.GET("/:dataset_id", r.datasetsHandler.GetDataset)
				datasets.PUT("/:dataset_id", r.datasetsHandler.UpdateDataset)
				datasets.GET("/:dataset_id/graph", r.datasetsHandler.GetKnowledgeGraph)
				datasets.GET("/:dataset_id/tags", r.datasetsHandler.ListTags)
				datasets.PUT("/:dataset_id/tags", r.datasetsHandler.RenameTag)
				datasets.DELETE("/:dataset_id/tags", r.datasetsHandler.RemoveTags)
				datasets.POST("/:dataset_id/embedding", r.datasetsHandler.RunEmbedding)
				datasets.POST("/:dataset_id/embedding/check", r.datasetsHandler.CheckEmbedding)
				datasets.POST("/:dataset_id/documents/batch-update-status", r.documentHandler.BatchUpdateDocumentStatus)
				datasets.GET("/:dataset_id/index", r.datasetsHandler.TraceIndex)
				datasets.POST("/:dataset_id/index", r.datasetsHandler.RunIndex)
				datasets.DELETE("/:dataset_id/index", r.datasetsHandler.DeleteIndex)
				datasets.DELETE("/:dataset_id/:index_type", r.datasetsHandler.DeleteIndex)
				//datasets.DELETE("/:dataset_id/graph", r.datasetsHandler.DeleteKnowledgeGraph)
				datasets.POST("", r.datasetsHandler.CreateDataset)
				datasets.DELETE("", r.datasetsHandler.DeleteDatasets)
				datasets.POST("/search", r.datasetsHandler.SearchDatasets)
				datasets.POST("/:dataset_id/search", r.datasetsHandler.SearchDataset)
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
				datasets.POST("/:dataset_id/documents", r.documentHandler.UploadDocuments)
				datasets.GET("/:dataset_id/documents/:document_id", r.documentHandler.DownloadDocument)
				datasets.PATCH("/:dataset_id/documents/:document_id", r.documentHandler.UpdateDatasetDocument)
				datasets.DELETE("/:dataset_id/documents", r.documentHandler.DeleteDocuments)
				datasets.POST("/:dataset_id/documents/:document_id/chunks", r.chunkHandler.AddChunk)

				// Dataset document chunk
				datasets.GET("/:dataset_id/documents/:document_id/chunks", r.chunkHandler.ListChunks)
				datasets.PATCH("/:dataset_id/documents/:document_id/chunks", r.chunkHandler.SwitchChunks)
				datasets.GET("/:dataset_id/documents/:document_id/chunks/:chunk_id", r.chunkHandler.Get)
				datasets.POST("/:dataset_id/chunks", r.chunkHandler.Parse)
				datasets.PATCH("/:dataset_id/documents/:document_id/chunks/:chunk_id", r.chunkHandler.UpdateChunk)
				datasets.POST("/:dataset_id/documents/parse", r.documentHandler.StartIngestionTask)
				datasets.GET("/ingestion/tasks", r.documentHandler.ListIngestionTasks)
				datasets.PUT("/ingestion/tasks", r.documentHandler.StopIngestionTasks)
				datasets.DELETE("/ingestion/tasks", r.documentHandler.RemoveIngestionTasks)
				//datasets.POST("/:dataset_id/documents/parse", r.documentHandler.ParseDocuments)
				//datasets.POST("/:dataset_id/documents/stop", r.documentHandler.StopParseDocuments)
				datasets.DELETE("/:dataset_id/chunks", r.chunkHandler.StopParsing)
				datasets.DELETE("/:dataset_id/documents/:document_id/chunks", r.chunkHandler.RemoveChunks)
				datasets.PUT("/:dataset_id/documents/:document_id/metadata/config", r.datasetsHandler.UpdateDocumentMetadataConfig)
				datasets.POST("/:dataset_id/metadata/update", r.documentHandler.MetadataBatchUpdate)
				datasets.PATCH("/:dataset_id/documents/metadatas", r.documentHandler.UpdateDocumentMetadatas)
			}

			// Search routes
			searches := v1.Group("/searches")
			{
				searches.GET("", r.searchHandler.ListSearches)
				searches.POST("", r.searchHandler.CreateSearch)
				searches.GET("/:search_id", r.searchHandler.GetSearch)
				searches.PUT("/:search_id", r.searchHandler.UpdateSearch)
				searches.DELETE("/:search_id", r.searchHandler.DeleteSearch)
				searches.POST("/:search_id/completion", r.searchHandler.Completion)
				searches.POST("/:search_id/completions", r.searchHandler.Completion)
			}

			files := v1.Group("/files")
			{
				files.POST("", r.fileHandler.UploadFile)
				files.GET("", r.fileHandler.ListFiles)
				files.DELETE("", r.fileHandler.DeleteFiles)
				files.POST("/move", r.fileHandler.MoveFiles)
				files.POST("/link-to-datasets", r.fileHandler.LinkToDatasets)
				files.GET("/:id/ancestors", r.fileHandler.GetFileAncestors)
				files.GET("/:id/parent", r.fileHandler.GetParentFolder)
				files.GET("/:id", r.fileHandler.Download)
				files.GET("/:id/versions", r.fileCommitHandler.GetFileVersionHistory)
			}

			// File routes
			file := authorized.Group("/v1/file")
			{
				file.GET("/root_folder", r.fileHandler.GetRootFolder)
				file.GET("/parent_folder", r.fileHandler.GetParentFolder)
				file.GET("/all_parent_folder", r.fileHandler.GetAllParentFolders)
			}

			// File commit routes — /folders/ takes folder_id directly
			commitFolders := v1.Group("/folders")
			{
				commitFolders.POST("/:folder_id/commits", r.fileCommitHandler.CreateCommit)
				commitFolders.GET("/:folder_id/commits", r.fileCommitHandler.ListCommits)
				commitFolders.GET("/:folder_id/commits/diff", r.fileCommitHandler.DiffCommits)
				commitFolders.GET("/:folder_id/commits/:commit_id", r.fileCommitHandler.GetCommit)
				commitFolders.GET("/:folder_id/commits/:commit_id/files", r.fileCommitHandler.ListCommitFiles)
				commitFolders.GET("/:folder_id/commits/:commit_id/tree", r.fileCommitHandler.GetCommitTree)
				commitFolders.GET("/:folder_id/commits/:commit_id/files/:file_id/content", r.fileCommitHandler.GetCommitFileContent)
				commitFolders.GET("/:folder_id/changes", r.fileCommitHandler.GetUncommittedChanges)
			}

			// /workspace/{workspace_id}/commits — alias for /folders/ (workspace_id == folder_id)
			commitWorkspace := v1.Group("/workspace")
			{
				commitWorkspace.POST("/:folder_id/commits", r.fileCommitHandler.CreateCommit)
				commitWorkspace.GET("/:folder_id/commits", r.fileCommitHandler.ListCommits)
				commitWorkspace.GET("/:folder_id/commits/diff", r.fileCommitHandler.DiffCommits)
				commitWorkspace.GET("/:folder_id/commits/:commit_id", r.fileCommitHandler.GetCommit)
				commitWorkspace.GET("/:folder_id/commits/:commit_id/files", r.fileCommitHandler.ListCommitFiles)
				commitWorkspace.GET("/:folder_id/commits/:commit_id/tree", r.fileCommitHandler.GetCommitTree)
				commitWorkspace.GET("/:folder_id/commits/:commit_id/files/:file_id/content", r.fileCommitHandler.GetCommitFileContent)
				commitWorkspace.GET("/:folder_id/changes", r.fileCommitHandler.GetUncommittedChanges)
			}

			// /datasets/{dataset_id}/commits — resolve dataset_id → folder_id via middleware
			commitDatasets := v1.Group("/datasets/:dataset_id")
			commitDatasets.Use(handler.CommitFolderResolver(r.fileCommitHandler, "datasets", "dataset_id"))
			{
				commitDatasets.POST("/commits", r.fileCommitHandler.CreateCommit)
				commitDatasets.GET("/commits", r.fileCommitHandler.ListCommits)
				commitDatasets.GET("/commits/diff", r.fileCommitHandler.DiffCommits)
				commitDatasets.GET("/commits/:commit_id", r.fileCommitHandler.GetCommit)
				commitDatasets.GET("/commits/:commit_id/files", r.fileCommitHandler.ListCommitFiles)
				commitDatasets.GET("/commits/:commit_id/tree", r.fileCommitHandler.GetCommitTree)
				commitDatasets.GET("/commits/:commit_id/files/:file_id/content", r.fileCommitHandler.GetCommitFileContent)
				commitDatasets.GET("/changes", r.fileCommitHandler.GetUncommittedChanges)
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
				message.GET("", r.memoryHandler.GetMessages)
				message.POST("", r.memoryHandler.AddMessage)
				message.DELETE("/:memory_message", r.memoryHandler.ForgetMessage)
				message.PUT("/:memory_message", r.memoryHandler.UpdateMessage)
				message.GET("/:memory_message/content", r.memoryHandler.GetMessageContent)
				message.GET("/search", r.memoryHandler.SearchMessage)
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
				provider.POST("/:provider_name/connection", r.providerHandler.CheckConnection)
				provider.GET("/:provider_name/instances/:instance_name/tasks", r.providerHandler.ListTasks)
				provider.GET("/:provider_name/instances/:instance_name/tasks/:task_id", r.providerHandler.ShowTask)
				provider.PUT("/:provider_name/instances/:instance_name", r.providerHandler.AlterProviderInstance)
				provider.DELETE("/:provider_name/instances", r.providerHandler.DropProviderInstance)
				provider.GET("/:provider_name/instances/:instance_name/models", r.providerHandler.ListInstanceModels)
				provider.PATCH("/:provider_name/instances/:instance_name/models/*model_name", r.providerHandler.EnableOrDisableModel)
				provider.POST("/:provider_name/instances/:instance_name/models", r.providerHandler.AddModel)
				provider.DELETE("/:provider_name/instances/:instance_name/models", r.providerHandler.DropInstanceModels)
				v1.POST("/chat/to_model", r.providerHandler.ChatToModel)
				v1.POST("/embeddings", r.providerHandler.EmbedText)
				v1.POST("/rerank", r.providerHandler.RerankDocument)
				v1.POST("/audio/transcriptions", r.providerHandler.TranscribeAudio)
				v1.POST("/audio/speech", r.providerHandler.AudioSpeech)
				v1.POST("/file/ocr", r.providerHandler.OCRFile)
				v1.POST("/file/parse", r.providerHandler.ParseFile)
			}

			model := v1.Group("/models")
			{
				// GET /models returns the tenant's added models across
				// all instances. Front-end useFetchAllAddedModels consumes this.
				model.GET("/", r.providerHandler.ListTenantAddedModels)
				model.PATCH("/", r.tenantHandler.SetModels)
				// Tenant default-model selection (used by the agent page's useFetchDefaultModels hook)
				model.GET("/default", r.tenantHandler.GetDefaultModels)
				model.PATCH("/default", r.tenantHandler.SetDefaultModels)
			}

			allModels := v1.Group("/all-models")
			{
				allModels.GET("", r.modelHandler.ListAllModels)
				allModels.GET("/:model_name", r.modelHandler.ShowModel)
			}

			// Agent routes
			agents := v1.Group("/agents")
			RegisterAgentRoutes(agents, r.agentHandler)

			// Plugin routes
			plugin := v1.Group("/plugin")
			{
				plugin.GET("/tools", r.pluginHandler.ListLLMTools)
			}

			// Component catalog — Phase 4 of
			// port-rag-flow-pipeline-to-go.md. Optional
			// ?category=ingestion,agent,shared filter; defaults to
			// all categories. The data source is
			// runtime.DefaultRegistry.
			if r.componentsHandler != nil {
				v1.GET("/components", r.componentsHandler.Get)
			}

			// Admin routes — Phase 6 per-tenant canvas runtime override.
			// RegisterAdminRuntimeRoutes lives in admin_routes.go; a nil
			// handler is tolerated and yields a no-op registration.
			admin := v1.Group("/admin")
			RegisterAdminRuntimeRoutes(admin, r.adminRuntimeHandler)

			connectors := v1.Group("/connectors")
			{
				connectors.GET("/", r.connectorHandler.ListConnectors)
				connectors.POST("/", r.connectorHandler.CreateConnector)
				connectors.POST("/google/oauth/web/start", r.connectorHandler.StartGoogleWebOAuth)
				connectors.POST("/google/oauth/web/result", r.connectorHandler.PollGoogleWebOAuthResult)
				connectors.POST("/box/oauth/web/start", r.connectorHandler.StartBoxWebOAuth)
				connectors.POST("/box/oauth/web/result", r.connectorHandler.PollBoxWebOAuthResult)
				connectors.GET("/:connector_id", r.connectorHandler.GetConnector)
				connectors.PATCH("/:connector_id", r.connectorHandler.UpdateConnector)
				connectors.GET("/:connector_id/logs", r.connectorHandler.ListLogs)
				connectors.DELETE("/:connector_id", r.connectorHandler.DeleteConnector)
				connectors.POST("/:connector_id/rebuild", r.connectorHandler.RebuildConnector)
				connectors.POST("/:connector_id/test", r.connectorHandler.TestConnector)
			}

			// Connector routes
			connector := authorized.Group("/v1/connector")
			{
				connector.GET("/list", r.connectorHandler.ListConnectors)
				connector.GET("/:connector_id", r.connectorHandler.GetConnector)
				connector.POST("/:connector_id/rebuild", r.connectorHandler.RebuildConnector)
			}

			// MCP server routes.
			mcp := v1.Group("/mcp")
			{
				mcp.POST("/servers", r.mcpHandler.CreateMCPServer)
				mcp.GET("/servers", r.mcpHandler.ListMCPServers)
				mcp.GET("/servers/:mcp_id", r.mcpHandler.GetMCPServer)
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

				// Variables/Settings
				system.GET("/variables", r.systemHandler.ListVariables)
				system.PUT("/variables", r.systemHandler.SetVariable)
				system.GET("/variables/:var_name", r.systemHandler.ShowVariable)

				// Environments
				system.GET("/environments", r.systemHandler.ListEnvironments)

				tokens := system.Group("/tokens")
				{
					// list tokens /api/v1/system/tokens GET
					tokens.GET("", r.systemHandler.ListAPIKeys)
					// create token /api/v1/system/tokens POST
					tokens.POST("", r.systemHandler.CreateKey)
					// delete token /api/v1/system/tokens/:key DELETE
					tokens.DELETE("/:key", r.systemHandler.DeleteKey)
				}

				keys := system.Group("/keys")
				{
					// list keys /api/v1/system/keys GET
					keys.GET("", r.systemHandler.ListAPIKeys)
					// create key /api/v1/system/keys POST
					keys.POST("", r.systemHandler.CreateKey)
					// delete key /api/v1/system/keys/:key DELETE
					keys.DELETE("/:key", r.systemHandler.DeleteKey)
				}
			}

			// Document routes
			doc := v1.Group("/document")
			{
				doc.POST("/list", r.documentHandler.ListDocuments)
				doc.POST("/metadata/summary", r.documentHandler.MetadataSummary)
				doc.POST("/set_meta", r.documentHandler.SetMeta)
				doc.POST("/delete_meta", r.documentHandler.DeleteMeta) // Internal API only for GO
			}

			// Chunk routes
			chunk := v1.Group("/chunk")
			{
				chunk.POST("/list", r.chunkHandler.List)
				chunk.POST("/update", r.chunkHandler.UpdateChunk) // Internal API only for GO
			}

			// Chat Channel
			chanChannel := v1.Group("/chat-channels")
			{
				chanChannel.POST("", r.chatChannelHandler.CreateChatChannel)
				chanChannel.GET("", r.chatChannelHandler.ListChatChannel)
				chanChannel.GET("/:channel_id", r.chatChannelHandler.GetChatChannel)
				chanChannel.PATCH("/:channel_id", r.chatChannelHandler.UpdateChatChannel)
				chanChannel.DELETE("/:channel_id", r.chatChannelHandler.DeleteChatChannel)
			}

			// Langfuse tracing keys
			langfuse := v1.Group("/langfuse")
			{
				langfuse.POST("/api-key", r.langfuseHandler.SetAPIKey)
				langfuse.PUT("/api-key", r.langfuseHandler.SetAPIKey)
				langfuse.GET("/api-key", r.langfuseHandler.GetAPIKey)
				langfuse.DELETE("/api-key", r.langfuseHandler.DeleteAPIKey)
			}

			// Dify retrieval routes
			dify := v1.Group("/dify")
			{
				dify.POST("/retrieval", r.difyRetrievalHandler.Retrieval)
				dify.GET("/retrieval", r.difyRetrievalHandler.Retrieval)
			}
		}
	}

	// Handle undefined routes
	engine.NoRoute(handler.HandleNoRoute)
}
