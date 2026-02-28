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
}

// NewRouter create router
func NewRouter(
	userHandler *handler.UserHandler,
	tenantHandler *handler.TenantHandler,
	documentHandler *handler.DocumentHandler,
	knowledgebaseHandler *handler.KnowledgebaseHandler,
	chunkHandler *handler.ChunkHandler,
	llmHandler *handler.LLMHandler,
	chatHandler *handler.ChatHandler,
	chatSessionHandler *handler.ChatSessionHandler,
) *Router {
	return &Router{
		userHandler:          userHandler,
		tenantHandler:        tenantHandler,
		documentHandler:      documentHandler,
		systemHandler:        handler.NewSystemHandler(),
		knowledgebaseHandler: knowledgebaseHandler,
		chunkHandler:         chunkHandler,
		llmHandler:           llmHandler,
		chatHandler:          chatHandler,
		chatSessionHandler:   chatSessionHandler,
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

	// System ping endpoint
	engine.GET("/v1/system/ping", r.systemHandler.Ping)

	// User login by email endpoint
	engine.POST("/v1/user/login", r.userHandler.LoginByEmail)
	// User logout endpoint
	engine.POST("/v1/user/logout", r.userHandler.Logout)
	// User info endpoint
	engine.GET("/v1/user/info", r.userHandler.Info)
	// User tenant info endpoint
	engine.GET("/v1/user/tenant_info", r.tenantHandler.TenantInfo)
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
		conversation := engine.Group("/v1/conversation")
		{
			conversation.POST("/set", r.chatSessionHandler.SetConversation)
			conversation.POST("/rm", r.chatSessionHandler.RemoveChatSessions)
			conversation.GET("/list", r.chatSessionHandler.ListChatSessions)
		}
	}

	// Handle undefined routes
	engine.NoRoute(handler.HandleNoRoute)
}
