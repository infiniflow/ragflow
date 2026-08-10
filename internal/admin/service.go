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
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/clickhouse"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/server"
	"ragflow/internal/server/config"
	servicepkg "ragflow/internal/service"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"regexp"
	"time"

	"go.uber.org/zap"
)

// Service admin service layer
type Service struct {
	userDAO             *dao.UserDAO
	licenseDAO          *dao.LicenseDAO
	timeRecordDAO       *dao.TimeRecordDAO
	systemSettingsDAO   *dao.SystemSettingsDAO
	tenantDAO           *dao.TenantDAO
	userTenantDAO       *dao.UserTenantDAO
	tenantLLMDAO        *dao.TenantLLMDAO
	fileDAO             *dao.FileDAO
	documentDAO         *dao.DocumentDAO
	taskDAO             *dao.TaskDAO
	kbDAO               *dao.KnowledgebaseDAO
	canvasDAO           *dao.UserCanvasDAO
	chatDAO             *dao.ChatDAO
	chatSessionDAO      *dao.ChatSessionDAO
	apiTokenDAO         *dao.APITokenDAO
	api4ConvDAO         *dao.API4ConversationDAO
	llmDAO              *dao.LLMDAO
	ingestionTaskDAO    *dao.IngestionTaskDAO
	ingestionTaskLogDao *dao.IngestionTaskLogDAO
	ingestionTaskSvc    *servicepkg.IngestionTaskService

	BillingProductDAO      *dao.BillingProductDAO
	BillingSubscriptionDAO *dao.BillingSubscriptionDAO
	MemoryDAO              *dao.MemoryDAO
	SearchDAO              *dao.SearchDAO
}

// NewService create admin service
func NewService() *Service {
	return &Service{
		userDAO:             dao.NewUserDAO(),
		licenseDAO:          dao.NewLicenseDAO(),
		timeRecordDAO:       dao.NewTimeRecordDAO(),
		systemSettingsDAO:   dao.NewSystemSettingsDAO(),
		tenantDAO:           dao.NewTenantDAO(),
		userTenantDAO:       dao.NewUserTenantDAO(),
		tenantLLMDAO:        dao.NewTenantLLMDAO(),
		fileDAO:             dao.NewFileDAO(),
		documentDAO:         dao.NewDocumentDAO(),
		taskDAO:             dao.NewTaskDAO(),
		kbDAO:               dao.NewKnowledgebaseDAO(),
		canvasDAO:           dao.NewUserCanvasDAO(),
		chatDAO:             dao.NewChatDAO(),
		chatSessionDAO:      dao.NewChatSessionDAO(),
		apiTokenDAO:         dao.NewAPITokenDAO(),
		api4ConvDAO:         dao.NewAPI4ConversationDAO(),
		llmDAO:              dao.NewLLMDAO(),
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDao: dao.NewIngestionTaskLogDAO(),
		ingestionTaskSvc:    servicepkg.NewIngestionTaskService(),

		BillingProductDAO:      dao.NewBillingProductDAO(),
		BillingSubscriptionDAO: dao.NewBillingSubscriptionDAO(),
		MemoryDAO:              dao.NewMemoryDAO(),
		SearchDAO:              dao.NewSearchDAO(),
	}
}

// Logout user logout
func (s *Service) Logout(ctx context.Context, user interface{}) error {
	// Invalidate token by setting it to INVALID_ prefix
	if u, ok := user.(*entity.User); ok {
		invalidToken := "INVALID_" + generateRandomHex(16)
		return s.userDAO.UpdateAccessToken(ctx, dao.DB, u, invalidToken)
	}
	return nil
}

// ListIngestionTasks list all ingestion tasks for admin user
func (s *Service) ListIngestionTasks(ctx context.Context) ([]map[string]interface{}, error) {
	return s.ingestionTaskSvc.ListAllForAdmin(ctx)
}

func (s *Service) RemoveIngestionTasks(ctx context.Context, tasks []string) ([]map[string]string, error) {
	return s.ingestionTaskSvc.RemoveMany(ctx, tasks, nil)
}

func (s *Service) StopIngestionTasks(ctx context.Context, tasks []string) ([]*entity.IngestionTask, error) {
	return s.ingestionTaskSvc.RequestStopMany(ctx, tasks, nil)
}

// GetUserByToken get user by access token
func (s *Service) GetUserByToken(ctx context.Context, token string) (*entity.User, error) {
	user, err := s.userDAO.GetByAccessToken(ctx, dao.DB, token)
	if err != nil {
		return nil, common.ErrInvalidToken
	}

	if user.IsSuperuser == nil || !*user.IsSuperuser {
		return nil, common.ErrNotAdmin
	}

	if user.IsActive != "1" {
		return nil, fmt.Errorf("user inactive")
	}

	return user, nil
}

// generateRandomHex generate random hex string
func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// ListUsers list all users
func (s *Service) ListUsers(ctx context.Context, pageIndex, pageSize int, name, status, sort, orderBy string) ([]map[string]interface{}, error) {
	users, _, err := s.userDAO.List(ctx, dao.DB, (pageIndex-1)*pageSize, pageSize, name, status, sort, orderBy)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		result = append(result, map[string]interface{}{
			"email":        user.Email,
			"nickname":     user.Nickname,
			"create_date":  user.CreateTime,
			"is_active":    user.IsActive,
			"is_superuser": user.IsSuperuser,
		})
	}
	return result, nil
}

// CreateUser create a new user
// Parameters:
//   - username: email address of the user
//   - password: encrypted password (base64 encoded RSA encrypted)
//   - role: user role ("user" or "admin")
//
// Returns:
//   - map[string]interface{}: user information without password
//   - error: error message
func (s *Service) CreateUser(ctx context.Context, username, password, role string) (map[string]interface{}, error) {
	emailRegex := regexp.MustCompile(`^[\w\._-]+@([\w_-]+\.)+[\w-]{2,}$`)
	if !emailRegex.MatchString(username) {
		return nil, fmt.Errorf("invalid email address: %s", username)
	}

	existUser, _ := s.userDAO.GetByEmail(ctx, dao.DB, username)
	if existUser != nil {
		return nil, fmt.Errorf("user '%s' already exists", username)
	}

	decryptedPassword, err := common.DecryptPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt password: %w", err)
	}

	hashedPassword, err := common.GenerateWerkzeugPasswordHash(decryptedPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	userID := utility.GenerateToken()
	accessToken := utility.GenerateToken()
	status := "1"
	loginChannel := "password"
	isSuperuser := role == "admin"

	user := &entity.User{
		ID:              userID,
		AccessToken:     &accessToken,
		Email:           username,
		Nickname:        "",
		Password:        &hashedPassword,
		Status:          &status,
		IsActive:        "1",
		IsAuthenticated: "1",
		IsAnonymous:     "0",
		LoginChannel:    &loginChannel,
		IsSuperuser:     &isSuperuser,
	}

	// Start transaction for creating user and related data
	tx := dao.DB.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Rollback helper function
	rollbackTx := func() {
		if rbErr := tx.Rollback(); rbErr.Error != nil {
			common.Error("failed to rollback transaction", rbErr.Error)
		}
	}

	// 1. Create user
	if err = tx.Create(user).Error; err != nil {
		rollbackTx()
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// 2. Create tenant (tenant_id = user_id)
	// tenant name = nickname + "'s Kingdom" (same as Python)
	tenantName := user.Nickname + "'s Kingdom"

	// Get default model IDs from config
	cfg := server.GetConfig()
	chatModel := ""
	embeddingModel := ""
	asrModel := ""
	vlmModel := ""
	rerankModel := ""
	var ttsModel *string = nil
	var ocrModel *string = nil
	parserIDs := "naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,paper:Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,email:Email"

	if cfg != nil {
		chatModel = cfg.GetDefaultChatModel().Name
		embeddingModel = cfg.GetDefaultEmbeddingModel().Name
		asrModel = cfg.GetDefaultASRModel().Name
		vlmModel = cfg.GetDefaultVisionModel().Name
		rerankModel = cfg.GetDefaultRerankModel().Name
		ttsModelStr := cfg.GetDefaultTTSModel().Name
		ttsModel = &ttsModelStr
		ocrModelStr := cfg.GetDefaultOCRModel().Name
		ocrModel = &ocrModelStr
	}

	tenantStatus := "1"
	tenant := &entity.Tenant{
		ID:        userID,
		Name:      &tenantName,
		LLMID:     chatModel,
		EmbdID:    embeddingModel,
		ASRID:     asrModel,
		Img2TxtID: vlmModel,
		RerankID:  rerankModel,
		TTSID:     ttsModel,
		OCRID:     ocrModel,
		ParserIDs: parserIDs,
		Credit:    512,
		Status:    &tenantStatus,
	}
	if err = tx.Create(tenant).Error; err != nil {
		rollbackTx()
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	// 3. Create user-tenant relation
	userTenantStatus := "1"
	userTenant := &entity.UserTenant{
		ID:        utility.GenerateToken(),
		UserID:    userID,
		TenantID:  userID,
		Role:      "owner",
		InvitedBy: userID,
		Status:    &userTenantStatus,
	}
	if err = tx.Create(userTenant).Error; err != nil {
		rollbackTx()
		return nil, fmt.Errorf("failed to create user-tenant relation: %w", err)
	}

	// 4. Create tenant LLM configurations
	tenantLLMs, err := s.getInitTenantLLM(ctx, userID)
	if err != nil {
		common.Warn("failed to get init tenant LLM configs", zap.Error(err))
		// Continue without LLM configs - not a critical error
	} else if len(tenantLLMs) > 0 {
		if err = tx.Create(&tenantLLMs).Error; err != nil {
			common.Warn("failed to create tenant LLM configs", zap.Error(err))
			// Continue without LLM configs - not a critical error
		}
	}

	// 5. Create root file folder
	fileID := utility.GenerateToken()
	fileLocation := ""
	file := &entity.File{
		ID:        fileID,
		ParentID:  fileID,
		TenantID:  userID,
		CreatedBy: userID,
		Name:      "/",
		Type:      "folder",
		Size:      0,
		Location:  &fileLocation,
	}
	if err = tx.Create(file).Error; err != nil {
		rollbackTx()
		return nil, fmt.Errorf("failed to create root file folder: %w", err)
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	common.Info("Create user success with tenant and related data", zap.String("username", username))

	return map[string]interface{}{
		"id":           user.ID,
		"email":        user.Email,
		"nickname":     user.Nickname,
		"is_active":    user.IsActive,
		"is_superuser": isSuperuser,
		"create_date":  user.CreateDate,
	}, nil
}

// getInitTenantLLM gets initial tenant LLM configurations
// This matches Python's get_init_tenant_llm function
func (s *Service) getInitTenantLLM(ctx context.Context, userID string) ([]*entity.TenantLLM, error) {
	cfg := server.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("config not initialized")
	}

	var tenantLLMs []*entity.TenantLLM

	// Get model configs from configuration
	modelConfigs := []config.ModelConfig{
		cfg.GetDefaultChatModel(),
		cfg.GetDefaultEmbeddingModel(),
		cfg.GetDefaultRerankModel(),
		cfg.GetDefaultASRModel(),
		cfg.GetDefaultVisionModel(),
		cfg.GetDefaultTTSModel(),
		cfg.GetDefaultOCRModel(),
	}

	// Track seen factories to avoid duplicates
	seenFactories := make(map[string]bool)
	var uniqueFactories []config.ModelConfig

	for _, mc := range modelConfigs {
		if mc.Factory == "" {
			continue
		}
		if !seenFactories[mc.Factory] {
			seenFactories[mc.Factory] = true
			uniqueFactories = append(uniqueFactories, mc)
		}
	}

	// Get LLMs for each unique factory
	for _, factoryConfig := range uniqueFactories {
		models, err := s.llmDAO.GetByFactory(ctx, dao.DB, factoryConfig.Factory)
		if err != nil {
			common.Warn("failed to get LLMs for factory", zap.String("factory", factoryConfig.Factory), zap.Error(err))
			continue
		}

		for _, model := range models {
			// Determine API key and base URL based on model type
			var apiKey, apiBase string
			switch model.ModelType {
			case entity.ModelTypeChat.String():
				apiKey = factoryConfig.APIKey
				apiBase = factoryConfig.BaseURL
			case entity.ModelTypeEmbedding.String():
				apiKey = cfg.GetDefaultEmbeddingModel().APIKey
				apiBase = cfg.GetDefaultEmbeddingModel().BaseURL
				if apiKey == "" {
					apiKey = factoryConfig.APIKey
				}
				if apiBase == "" {
					apiBase = factoryConfig.BaseURL
				}
			case entity.ModelTypeRerank.String():
				apiKey = cfg.GetDefaultRerankModel().APIKey
				apiBase = cfg.GetDefaultRerankModel().BaseURL
				if apiKey == "" {
					apiKey = factoryConfig.APIKey
				}
				if apiBase == "" {
					apiBase = factoryConfig.BaseURL
				}
			case entity.ModelTypeSpeech2Text.String():
				apiKey = cfg.GetDefaultASRModel().APIKey
				apiBase = cfg.GetDefaultASRModel().BaseURL
				if apiKey == "" {
					apiKey = factoryConfig.APIKey
				}
				if apiBase == "" {
					apiBase = factoryConfig.BaseURL
				}
			case entity.ModelTypeImage2Text.String():
				apiKey = cfg.GetDefaultVisionModel().APIKey
				apiBase = cfg.GetDefaultVisionModel().BaseURL
				if apiKey == "" {
					apiKey = factoryConfig.APIKey
				}
				if apiBase == "" {
					apiBase = factoryConfig.BaseURL
				}
			default:
				apiKey = factoryConfig.APIKey
				apiBase = factoryConfig.BaseURL
			}

			maxTokens := int64(8192)
			if model.MaxTokens > 0 {
				maxTokens = model.MaxTokens
			}

			llmName := model.LLMName
			modelType := model.ModelType
			tenantLLM := &entity.TenantLLM{
				TenantID:   userID,
				LLMFactory: factoryConfig.Factory,
				LLMName:    &llmName,
				ModelType:  &modelType,
				APIKey:     &apiKey,
				APIBase:    &apiBase,
				MaxTokens:  maxTokens,
				Status:     "1",
			}
			tenantLLMs = append(tenantLLMs, tenantLLM)
		}
	}

	// Remove duplicates based on (tenant_id, llm_factory, llm_name)
	seen := make(map[string]bool)
	var uniqueLLMs []*entity.TenantLLM
	for _, tenantLLM := range tenantLLMs {
		key := fmt.Sprintf("%s|%s|%s", tenantLLM.TenantID, tenantLLM.LLMFactory, *tenantLLM.LLMName)
		if !seen[key] {
			seen[key] = true
			uniqueLLMs = append(uniqueLLMs, tenantLLM)
		}
	}

	return uniqueLLMs, nil
}

// GetUserDetails get user details
func (s *Service) GetUserDetails(username string) (map[string]interface{}, error) {
	// Query user by email/username
	var user entity.User
	err := dao.DB.Where("email = ?", username).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	return map[string]interface{}{
		"id":           user.ID,
		"email":        user.Email,
		"nickname":     user.Nickname,
		"is_active":    user.IsActive,
		"is_superuser": user.IsSuperuser,
		"create_time":  user.CreateTime,
		"update_time":  user.UpdateTime,
	}, nil
}

// DeleteUserResult result of delete user operation
type DeleteUserResult struct {
	Username        string   `json:"username"`
	TenantLLMCount  int      `json:"tenant_llm_count"`
	LangfuseCount   int      `json:"langfuse_count"`
	MetadataTable   string   `json:"metadata_table"`
	TenantCount     int      `json:"tenant_count"`
	UserTenantCount int      `json:"user_tenant_count"`
	UserCount       int      `json:"user_count"`
	DeletedDetails  []string `json:"deleted_details"`
}

// DeleteUser delete user with cascade delete of all related data
// Parameters:
//   - username: email address of the user to delete
//
// Returns:
//   - *DeleteUserResult
//   - error: error message
func (s *Service) DeleteUser(ctx context.Context, username string) (*DeleteUserResult, error) {
	result := &DeleteUserResult{
		Username:       username,
		DeletedDetails: []string{fmt.Sprintf("Drop user: %s", username)},
	}
	userList, err := s.userDAO.ListByEmail(ctx, dao.DB, username)
	if err != nil || len(userList) == 0 {
		return nil, fmt.Errorf("user '%s' not found", username)
	}

	if len(userList) > 1 {
		return nil, fmt.Errorf("exist more than 1 user: %s", username)
	}

	user := userList[0]

	// Check if user is active - cannot delete active users
	if user.IsActive == "1" {
		return nil, fmt.Errorf("user '%s' is active and can't be deleted. Please deactivate the user first", username)
	}

	// Check if user is superuser - cannot delete admin accounts
	if user.IsSuperuser != nil && *user.IsSuperuser {
		return nil, fmt.Errorf("user '%s' is admin account and cannot be deleted", username)
	}

	// Get user-tenant relations
	tenants, err := s.userTenantDAO.GetByUserIDAll(ctx, dao.DB, user.ID)
	if err != nil {
		common.Warn("failed to get user-tenant relations", zap.Error(err))
	}

	// Find owned tenant (role = "owner")
	var ownedTenantID string
	for _, t := range tenants {
		if t.Role == "owner" {
			ownedTenantID = t.TenantID
			break
		}
	}

	// Start transaction for cascade delete
	tx := dao.DB.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Rollback helper function
	rollbackTx := func() {
		if rbErr := tx.Rollback(); rbErr.Error != nil {
			common.Error("failed to rollback transaction", rbErr.Error)
		}
	}

	result.DeletedDetails = append(result.DeletedDetails, "Start to delete owned tenant.")
	// Delete owned tenant data
	if ownedTenantID != "" {
		// 1. Get knowledge base IDs
		kbIDs, err := s.kbDAO.GetKBIDsByTenantIDSimple(ctx, tx, ownedTenantID)
		if err != nil {
			common.Warn("failed to get knowledge base IDs", zap.Error(err))
		}

		if len(kbIDs) > 0 {
			// 2. Get document IDs
			docIDs, err := s.documentDAO.GetAllDocIDsByKBIDs(ctx, tx, kbIDs)
			if err != nil {
				common.Warn("failed to get document IDs", zap.Error(err))
			}

			// 3. Delete tasks by document IDs
			if len(docIDs) > 0 {
				docIDList := make([]string, len(docIDs))
				for i, d := range docIDs {
					docIDList[i] = d["id"]
				}
				if delErr := tx.Unscoped().Where("doc_id IN ?", docIDList).Delete(&entity.Task{}); delErr.Error != nil {
					common.Warn("failed to delete tasks", zap.Error(delErr.Error))
				}
			}

			// 4. Delete documents
			if delErr := tx.Unscoped().Where("kb_id IN ?", kbIDs).Delete(&entity.Document{}); delErr.Error != nil {
				common.Warn("failed to delete documents", zap.Error(delErr.Error))
			}

			// 5. Delete knowledge bases
			if delErr := tx.Unscoped().Where("id IN ?", kbIDs).Delete(&entity.Knowledgebase{}); delErr.Error != nil {
				common.Warn("failed to delete knowledge bases", zap.Error(delErr.Error))
			}
		}

		// 6. Delete files
		if delErr := tx.Unscoped().Where("tenant_id = ?", ownedTenantID).Delete(&entity.File{}); delErr.Error != nil {
			common.Warn("failed to delete files", zap.Error(delErr.Error))
		}

		// 7. Delete user canvas (agents)
		if delErr := tx.Unscoped().Where("user_id = ?", ownedTenantID).Delete(&entity.UserCanvas{}); delErr.Error != nil {
			common.Warn("failed to delete user canvas", zap.Error(delErr.Error))
		}

		// 8. Get dialog IDs
		var dialogIDs []string
		if pluckErr := tx.Model(&entity.Chat{}).Where("tenant_id = ?", ownedTenantID).Pluck("id", &dialogIDs); pluckErr.Error != nil {
			common.Warn("failed to get dialog IDs", zap.Error(pluckErr.Error))
		}

		// 9. Delete chat sessions
		if len(dialogIDs) > 0 {
			if delErr := tx.Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&entity.ChatSession{}); delErr.Error != nil {
				common.Warn("failed to delete chat sessions", zap.Error(delErr.Error))
			}
		}

		// 10. Delete chats/dialogs
		if delErr := tx.Unscoped().Where("tenant_id = ?", ownedTenantID).Delete(&entity.Chat{}); delErr.Error != nil {
			common.Warn("failed to delete chats", zap.Error(delErr.Error))
		}

		// 11. Delete API tokens
		if delErr := tx.Unscoped().Where("tenant_id = ?", ownedTenantID).Delete(&entity.APIToken{}); delErr.Error != nil {
			common.Warn("failed to delete API tokens", zap.Error(delErr.Error))
		}

		// 12. Delete API4Conversations
		if len(dialogIDs) > 0 {
			if delErr := tx.Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&entity.API4Conversation{}); delErr.Error != nil {
				common.Warn("failed to delete API4Conversations", zap.Error(delErr.Error))
			}
		}

		var tenantLLMCount int64
		tx.Model(&entity.TenantLLM{}).Where("tenant_id = ?", ownedTenantID).Count(&tenantLLMCount)
		result.TenantLLMCount = int(tenantLLMCount)
		result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted %d tenant-LLM records.", tenantLLMCount))

		result.LangfuseCount = 0
		result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted %d langfuse records.", result.LangfuseCount))

		metadataTableName := fmt.Sprintf("ragflow_doc_meta_%s", ownedTenantID[:32])
		result.MetadataTable = metadataTableName
		result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted metadata table %s.", metadataTableName))

		// 13. Delete tenant LLM configurations
		if delErr := tx.Unscoped().Where("tenant_id = ?", ownedTenantID).Delete(&entity.TenantLLM{}); delErr.Error != nil {
			common.Warn("failed to delete tenant LLM", zap.Error(delErr.Error))
		}

		var tenantCount int64
		tx.Model(&entity.Tenant{}).Where("id = ?", ownedTenantID).Count(&tenantCount)
		result.TenantCount = int(tenantCount)
		// 14. Delete tenant
		if delErr := tx.Unscoped().Where("id = ?", ownedTenantID).Delete(&entity.Tenant{}); delErr.Error != nil {
			common.Warn("failed to delete tenant", zap.Error(delErr.Error))
		}
		result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted %d tenant.", result.TenantCount))
	}

	var userTenantCount int64
	tx.Model(&entity.UserTenant{}).Where("user_id = ?", user.ID).Count(&userTenantCount)
	result.UserTenantCount = int(userTenantCount)

	// 15. Delete user-tenant relations
	if delErr := tx.Unscoped().Where("user_id = ?", user.ID).Delete(&entity.UserTenant{}); delErr.Error != nil {
		common.Warn("failed to delete user-tenant relations", zap.Error(delErr.Error))
	}
	result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted %d user-tenant records.", result.UserTenantCount))

	result.UserCount = 1
	// 16. Finally, hard delete user
	if delErr := tx.Unscoped().Where("id = ?", user.ID).Delete(&entity.User{}); delErr.Error != nil {
		rollbackTx()
		return nil, fmt.Errorf("failed to delete user: %w", delErr.Error)
	}
	result.DeletedDetails = append(result.DeletedDetails, fmt.Sprintf("- Deleted %d user.", result.UserCount))

	// Commit transaction
	if commitErr := tx.Commit(); commitErr.Error != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", commitErr.Error)
	}

	result.DeletedDetails = append(result.DeletedDetails, "Delete done!")

	common.Info("Delete user success with all related data", zap.String("username", username))

	return result, nil
}

// ChangePassword change user password
// Parameters:
//   - username: email address of the user
//   - newPassword: new encrypted password (base64 encoded RSA encrypted)
//
// Returns:
//   - error: error message
func (s *Service) ChangePassword(ctx context.Context, username, newPassword string) error {
	userList, err := s.userDAO.ListByEmail(ctx, dao.DB, username)
	if err != nil || len(userList) == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}

	if len(userList) > 1 {
		return fmt.Errorf("exist more than 1 user: %s", username)
	}

	user := userList[0]

	decryptedPassword, err := common.DecryptPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to decrypt password: %w", err)
	}

	if user.Password != nil && common.CheckWerkzeugPassword(decryptedPassword, *user.Password) {
		return nil
	}

	hashedPassword, err := common.GenerateWerkzeugPasswordHash(decryptedPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user.Password = &hashedPassword

	if err = s.userDAO.Update(ctx, dao.DB, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// UpdateUserActivateStatus update user activate status
// Parameters:
//   - username: email address of the user
//   - isActive: true to activate, false to deactivate
//
// Returns:
//   - error: error message
func (s *Service) UpdateUserActivateStatus(ctx context.Context, username string, isActive bool) error {
	userList, err := s.userDAO.ListByEmail(ctx, dao.DB, username)
	if err != nil || len(userList) == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}

	if len(userList) > 1 {
		return fmt.Errorf("exist more than 1 user: %s", username)
	}

	user := userList[0]

	targetStatus := "0"
	if isActive {
		targetStatus = "1"
	}

	if user.IsActive == targetStatus {
		return nil
	}

	user.IsActive = targetStatus

	if err = s.userDAO.Update(ctx, dao.DB, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// GrantAdmin grant admin privileges
// Parameters:
//   - username: email address of the user
//
// Returns:
//   - error: error message
func (s *Service) GrantAdmin(ctx context.Context, username string) error {
	userList, err := s.userDAO.ListByEmail(ctx, dao.DB, username)
	if err != nil || len(userList) == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}

	if len(userList) > 1 {
		return fmt.Errorf("exist more than 1 user: %s", username)
	}

	user := userList[0]

	if user.IsSuperuser != nil && *user.IsSuperuser {
		return nil
	}

	isSuperuser := true
	user.IsSuperuser = &isSuperuser

	if err = s.userDAO.Update(ctx, dao.DB, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// RevokeAdmin revoke admin privileges
// Parameters:
//   - username: email address of the user
//
// Returns:
//   - error: error message
func (s *Service) RevokeAdmin(ctx context.Context, username string) error {
	userList, err := s.userDAO.ListByEmail(ctx, dao.DB, username)
	if err != nil || len(userList) == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}

	if len(userList) > 1 {
		return fmt.Errorf("exist more than 1 user: %s", username)
	}

	user := userList[0]

	if user.IsSuperuser == nil || !*user.IsSuperuser {
		return nil
	}

	isSuperuser := false
	user.IsSuperuser = &isSuperuser

	if err = s.userDAO.Update(ctx, dao.DB, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// GetUserDatasets get user datasets
func (s *Service) GetUserDatasets(username string) ([]map[string]interface{}, error) {
	// TODO: Implement get user datasets
	return []map[string]interface{}{}, nil
}

// GetUserAgents get user agents
func (s *Service) GetUserAgents(username string) ([]map[string]interface{}, error) {
	// TODO: Implement get user agents
	return []map[string]interface{}{}, nil
}

// API Key methods

// ListUserAPITokens get user API keys
func (s *Service) ListUserAPITokens(ctx context.Context, username string) ([]map[string]interface{}, error) {
	// 1. Get user details
	user, err := s.userDAO.GetByEmail(ctx, dao.DB, username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// 2. Get user's tenants
	userTenants, err := s.userTenantDAO.GetByUserID(ctx, dao.DB, user.ID)
	if err != nil || len(userTenants) == 0 {
		return nil, fmt.Errorf("tenant not found")
	}

	tenantID := userTenants[0].TenantID

	// 3. Get API tokens by tenant ID
	tokens, err := s.apiTokenDAO.GetByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API tokens: %w", err)
	}

	// 4. Convert to map slice
	result := make([]map[string]interface{}, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, map[string]interface{}{
			"tenant_id":   token.TenantID,
			"token":       token.Token,
			"beta":        token.Beta,
			"dialog_id":   token.DialogID,
			"source":      token.Source,
			"create_time": token.CreateTime,
			"create_date": token.CreateDate,
			"update_time": token.UpdateTime,
			"update_date": token.UpdateDate,
		})
	}

	return result, nil
}

// GenerateUserAPIToken generate API key for user
func (s *Service) GenerateUserAPIToken(ctx context.Context, username string) (map[string]interface{}, error) {
	// 1. Get user details
	user, err := s.userDAO.GetByEmail(ctx, dao.DB, username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// 2. Get user's tenants
	userTenants, err := s.userTenantDAO.GetByUserID(ctx, dao.DB, user.ID)
	if err != nil || len(userTenants) == 0 {
		return nil, fmt.Errorf("tenant not found")
	}

	tenantID := userTenants[0].TenantID

	// 3. Generate API token
	key := utility.GenerateAPIToken()
	beta := utility.GenerateBetaAPIToken()

	apiToken := &entity.APIToken{
		TenantID: tenantID,
		Token:    key,
		Beta:     &beta,
	}

	// 4. Save API token
	if err = s.apiTokenDAO.Create(ctx, dao.DB, apiToken); err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	return map[string]interface{}{
		"tenant_id":   tenantID,
		"token":       key,
		"beta":        beta,
		"create_time": apiToken.CreateTime,
		"create_date": apiToken.CreateDate,
		"update_time": apiToken.UpdateTime,
		"update_date": apiToken.UpdateDate,
	}, nil
}

// DeleteUserAPIToken delete user API key
func (s *Service) DeleteUserAPIToken(ctx context.Context, username, key string) error {
	// 1. Get user details
	user, err := s.userDAO.GetByEmail(ctx, dao.DB, username)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// 2. Get user's tenants
	userTenants, err := s.userTenantDAO.GetByUserID(ctx, dao.DB, user.ID)
	if err != nil || len(userTenants) == 0 {
		return fmt.Errorf("tenant not found")
	}

	tenantID := userTenants[0].TenantID

	// 3. Delete API token
	rowsAffected, err := s.apiTokenDAO.DeleteByTenantIDAndToken(ctx, dao.DB, tenantID, key)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("API key not found or could not be deleted")
	}

	return nil
}

type ServiceStatus struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Elapsed string `json:"elapsed"`
	Message string `json:"message"`
}

func newServiceStatus(typeStr, nameStr, statusStr string, startTime time.Time, messageStr string) ServiceStatus {
	return ServiceStatus{
		Type:    typeStr,
		Name:    nameStr,
		Status:  statusStr,
		Elapsed: fmt.Sprintf("%.1d", time.Since(startTime).Milliseconds()),
		Message: messageStr,
	}
}

// ListServices get all services
func (s *Service) ListServices(ctx context.Context) ([]ServiceStatus, error) {

	var results []ServiceStatus

	globalConfig := server.GetConfig()
	// Database
	databaseType := globalConfig.DatabaseType()
	switch databaseType {
	case "mysql":
		mysqlStatus := s.getMySQLStatus(ctx)
		results = append(results, mysqlStatus)
	default:
		results = append(results, newServiceStatus("database", databaseType, "not available", time.Now(), "not supported database type"))
	}

	// Doc engine
	docEngineImpl := engine.Get()
	err := docEngineImpl.Ping(ctx)
	if err == nil {
		results = append(results, newServiceStatus("doc_engine", docEngineImpl.GetType(), "alive", time.Now(), ""))
	} else {
		results = append(results, newServiceStatus("doc_engine", docEngineImpl.GetType(), "timeout", time.Now(), err.Error()))
	}

	// storage engine
	storageImpl := storage.GetStorageFactory().GetStorage()
	storageHealth := storageImpl.Health(ctx)
	if storageHealth {
		results = append(results, newServiceStatus("storage_engine", storageImpl.Type(), "alive", time.Now(), ""))
	} else {
		results = append(results, newServiceStatus("storage_engine", storageImpl.Type(), "timeout", time.Now(), ""))
	}

	// cache engine
	cacheType := globalConfig.CacheEngineType()
	switch cacheType {
	case "redis":
		mysqlStatus := s.getRedisInfo(ctx)
		results = append(results, mysqlStatus)
	default:
		results = append(results, newServiceStatus("database", databaseType, "not available", time.Now(), "not supported database type"))
	}

	// message queue
	messageQueueImpl := engine.GetMessageQueueEngine()
	messageQueueStatus := messageQueueImpl.CheckStatus()
	results = append(results, newServiceStatus("message_queue", messageQueueImpl.Type(), messageQueueStatus, time.Now(), ""))

	results = append(results, s.GetEEServicesStatus()...)

	serverList := GlobalServerStore.ListInfos()
	for _, serverStatus := range serverList {
		now := time.Now()
		serverItem := newServiceStatus(string(serverStatus.ServerType), serverStatus.ServerName, "", serverStatus.Timestamp, "")
		// the difference between now and serverStatus.Timestamp is less than 5 seconds, then the server is alive
		if now.Sub(serverStatus.Timestamp) < 45*time.Second {
			serverItem.Status = "alive"
		} else {
			serverItem.Status = "timeout"
		}
		results = append(results, serverItem)
	}

	return results, nil
}

// GetServicesByType get services by type
func (s *Service) GetServicesByType(serviceType string) ([]map[string]interface{}, error) {
	return nil, errors.New("get_services_by_type: not implemented")
}

// GetServiceDetails get service details
func (s *Service) GetServiceDetails(configDict map[string]interface{}) ([]ServiceStatus, error) {
	//serviceType, _ := configDict["service_type"].(string)
	//name, _ := configDict["name"].(string)
	//ctx := context.Background()
	//
	//// Call detail function based on service type
	//switch serviceType {
	//case "meta_data":
	//	return s.getMySQLStatus(ctx), nil
	//case "cache":
	//	return s.getRedisInfo(ctx), nil
	//case "message_queue":
	//	host := configDict["host"].(string)
	//	port := configDict["port"].(int)
	//	return s.checkNatsAlive(name, host, port)
	//case "retrieval":
	//	// Check the extra.retrieval_type to determine which retrieval service
	//	if extra, ok := configDict["extra"].(map[string]interface{}); ok {
	//		if retrievalType, ok := extra["retrieval_type"].(string); ok {
	//			if retrievalType == "infinity" {
	//				return s.getInfinityStatus(name)
	//			}
	//		}
	//	}
	//	return s.getESClusterStats(name)
	//case "ragflow_server":
	//	return s.checkRAGFlowServerAlive(name)
	//case "file_store":
	//	return s.checkMinioAlive(name)
	//case "olap":
	//	return s.checkOlapAlive(name)
	//case "tracing":
	//	return s.checkTracingAlive(name)
	//default:
	//	return nil, nil
	//}
	return nil, nil
}

// getMySQLStatus gets MySQL service status
func (s *Service) getMySQLStatus(ctx context.Context) ServiceStatus {

	serviceType := "database"
	name := "mysql"
	startTime := time.Now()

	sqlDB, err := dao.DB.DB()
	if err != nil {
		return newServiceStatus(serviceType, name, "not connected", startTime, err.Error())
	}

	// Execute SELECT 1 to check connectivity
	err = sqlDB.PingContext(ctx)
	if err != nil {
		return newServiceStatus(serviceType, name, "timeout", startTime, err.Error())
	}

	return newServiceStatus(serviceType, name, "alive", startTime, "")
}

// getRedisInfo gets Redis service info
func (s *Service) getRedisInfo(ctx context.Context) ServiceStatus {

	serviceType := "cache"
	name := "redis"

	startTime := time.Now()

	redisClient := redis.Get()
	if redisClient.Health(ctx) {
		return newServiceStatus(serviceType, name, "alive", startTime, "")
	}

	return newServiceStatus(serviceType, name, "timeout", startTime, "Redis health check failed")
}

// getESClusterStats gets Elasticsearch cluster stats
func (s *Service) getESClusterStats(serviceType string) map[string]interface{} {
	name := "elasticsearch"
	startTime := time.Now()

	cfg := server.GetConfig()
	if cfg == nil || !cfg.IsElasticConfigured() {
		return map[string]interface{}{
			"type":    serviceType,
			"name":    name,
			"status":  "timeout",
			"elapsed": fmt.Sprintf("%.1d", time.Since(startTime).Milliseconds()),
			"message": "error: Elasticsearch configuration not found",
		}
	}

	// Create ES engine and get cluster stats
	esEngine, err := elasticsearch.NewEngine(cfg.GetElasticsearchConfig())
	if err != nil {
		return map[string]interface{}{
			"type":    serviceType,
			"name":    name,
			"status":  "timeout",
			"elapsed": fmt.Sprintf("%.1d", time.Since(startTime).Milliseconds()),
			"message": fmt.Sprintf("error: %s", err.Error()),
		}
	}
	defer esEngine.Close()

	clusterStats, err := esEngine.GetClusterStats()
	if err != nil {
		return map[string]interface{}{
			"type":    serviceType,
			"name":    name,
			"status":  "timeout",
			"elapsed": fmt.Sprintf("%.1d", time.Since(startTime).Milliseconds()),
			"message": fmt.Sprintf("error: %s", err.Error()),
		}
	}

	return map[string]interface{}{
		"type":    serviceType,
		"name":    name,
		"status":  "alive",
		"elapsed": fmt.Sprintf("%.1d", time.Since(startTime).Milliseconds()),
		"message": clusterStats,
	}
}

// getInfinityStatus gets Infinity service status
func (s *Service) getInfinityStatus(serviceType string) map[string]interface{} {
	name := "infinity"
	startTime := time.Now()
	// TODO: Implement actual Infinity health check
	return map[string]interface{}{
		"type":    serviceType,
		"name":    name,
		"status":  "unknown",
		"elapsed": fmt.Sprintf("%.1d", time.Since(startTime).Milliseconds()),
		"message": "Infinity health check not implemented",
	}
}

// checkTaskExecutorAlive checks if task executor is alive
func (s *Service) checkTaskExecutorAlive(name string) (map[string]interface{}, error) {
	// TODO: Implement actual task executor health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
		"message":      "Task executor health check not implemented",
	}, nil
}

// checkNatsAlive checks if NATS is alive
func (s *Service) checkNatsAlive(name string, ip string, port int) (map[string]interface{}, error) {

	msgQueueEngine := engine.GetMessageQueueEngine()
	if msgQueueEngine == nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "Message queue engine not initialized",
		}, nil
	}

	status := msgQueueEngine.CheckStatus()

	return map[string]interface{}{
		"service_name": name,
		"status":       status,
	}, nil
}

// checkTracingAlive checks if tracing is alive
func (s *Service) checkTracingAlive(name string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
		"message":      "Tracing health check not implemented",
	}, nil
}

// checkOlapAlive checks if ClickHouse is alive
func (s *Service) checkOlapAlive(name string) (map[string]interface{}, error) {
	clickhouseDriver := clickhouse.GetDriver()
	if clickhouseDriver == nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "unknown",
		}, nil
	}

	status, err := clickhouseDriver.Status()
	if err != nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"message":      fmt.Sprintf("error: %s", err.Error()),
		}, nil
	}

	return map[string]interface{}{
		"service_name": name,
		"status":       "alive",
		"message":      status,
	}, nil
}

// ShutdownService shutdown service
func (s *Service) ShutdownService(serviceName string) (map[string]interface{}, error) {
	// TODO: Implement with proper service manager
	return map[string]interface{}{
		"command":      "shutdown service",
		"service_name": serviceName,
		"error":        "shutdown service not implemented",
	}, nil
}

// StartService start service
func (s *Service) StartService(serviceName string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":      "start service",
		"service_name": serviceName,
		"error":        "command 'start service' isn't implemented",
	}, nil
}

// RestartService restart service
func (s *Service) RestartService(serviceName string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":      "restart service",
		"service_name": serviceName,
		"error":        "command 'restart service' isn't implemented",
	}, nil
}

// AdminException admin exception error
type AdminException struct {
	Message string
	Code    int
}

// Error implement error interface
func (e *AdminException) Error() string {
	return e.Message
}

// NewAdminException create admin exception
func NewAdminException(message string) *AdminException {
	return &AdminException{
		Message: message,
		Code:    400,
	}
}

// GetVariable get variable by name
// Returns the exact system setting with the given name, or settings matching the
// given name prefix when an exact setting does not exist.
func (s *Service) GetVariable(ctx context.Context, varName string) ([]map[string]interface{}, error) {
	settings, err := s.systemSettingsDAO.GetByName(ctx, dao.DB, varName)
	if err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		settings, err = s.systemSettingsDAO.GetByNamePrefix(ctx, dao.DB, varName)
		if err != nil {
			return nil, err
		}
		if len(settings) == 0 {
			return nil, NewAdminException("Can't get setting: " + varName)
		}
	}
	return common.FormatSystemSettings(settings), nil
}

// ListAllVariables list all variables
// Returns all system settings from database
func (s *Service) ListAllVariables(ctx context.Context) ([]map[string]interface{}, error) {
	settings, err := s.systemSettingsDAO.GetAll(ctx, dao.DB)
	if err != nil {
		return nil, err
	}

	return common.FormatSystemSettings(settings), nil
}

// SetVariable set variable
// Creates or updates a system setting
// If the setting exists, updates it; otherwise creates a new one
func (s *Service) SetVariable(ctx context.Context, varName, varValue string) error {
	settings, err := s.systemSettingsDAO.GetByName(ctx, dao.DB, varName)
	if err != nil {
		return err
	}

	if len(settings) == 1 {
		setting := &settings[0]
		if err = common.ValidateSystemSettingValue(*setting, varValue); err != nil {
			return err
		}
		setting.Value = varValue
		return s.systemSettingsDAO.UpdateByName(ctx, dao.DB, varName, setting)
	} else if len(settings) > 1 {
		return NewAdminException("Can't update more than 1 setting: " + varName)
	}

	dataType := common.InferSystemSettingDataType(varName)
	newSetting := &entity.SystemSettings{
		Name:     varName,
		Value:    varValue,
		Source:   "admin",
		DataType: dataType,
	}
	if err = common.ValidateSystemSettingValue(*newSetting, varValue); err != nil {
		return err
	}
	return s.systemSettingsDAO.Create(ctx, dao.DB, newSetting)
}

// Config methods

// ListAllConfigs list all configs
// Returns all service configurations from the config file
func (s *Service) ListAllConfigs() ([]map[string]interface{}, error) {
	result, err := server.GetAllConfigs()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Environment methods

// ListEnvironments list all environments
func (s *Service) ListEnvironments() ([]map[string]interface{}, error) {
	result := make([]map[string]interface{}, 0)

	// DOC_ENGINE
	docEngine := common.GetEnv(common.EnvDocEngine)
	if docEngine == "" {
		docEngine = "elasticsearch"
	}
	result = append(result, map[string]interface{}{
		"env":   "DOC_ENGINE",
		"value": docEngine,
	})

	// DEFAULT_SUPERUSER_EMAIL
	defaultSuperuserEmail := common.GetEnv(common.EnvDefaultSuperuserEmail)
	if defaultSuperuserEmail == "" {
		defaultSuperuserEmail = "admin@ragflow.io"
	}
	result = append(result, map[string]interface{}{
		"env":   common.EnvDefaultSuperuserEmail,
		"value": defaultSuperuserEmail,
	})

	// DB_TYPE
	dbType := common.GetEnv(common.EnvDBType)
	if dbType == "" {
		dbType = "mysql"
	}
	result = append(result, map[string]interface{}{
		"env":   "DB_TYPE",
		"value": dbType,
	})

	// DEVICE
	device := common.GetEnv(common.EnvDevice)
	if device == "" {
		device = "cpu"
	}
	result = append(result, map[string]interface{}{
		"env":   "DEVICE",
		"value": device,
	})

	// STORAGE_IMPL
	storageImpl := common.GetEnv(common.EnvStorageImpl)
	if storageImpl == "" {
		storageImpl = "MINIO"
	}
	result = append(result, map[string]interface{}{
		"env":   common.EnvStorageImpl,
		"value": storageImpl,
	})

	return result, nil
}

// Version methods

// GetVersion get RAGFlow version
func (s *Service) GetVersion() (string, string) {
	return common.GetRAGFlowVersion(), common.GetRAGFlowType()
}

// Sandbox methods

// ListSandboxProviders list sandbox providers
func (s *Service) ListSandboxProviders() ([]map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return []map[string]interface{}{}, nil
}

// GetSandboxProviderSchema get sandbox provider schema
func (s *Service) GetSandboxProviderSchema(providerID string) (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{}, nil
}

// GetSandboxConfig get sandbox config
func (s *Service) GetSandboxConfig() (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{}, nil
}

// SetSandboxConfig set sandbox config
func (s *Service) SetSandboxConfig(providerType string, config map[string]interface{}, setActive bool) (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{
		"provider_type": providerType,
		"config":        config,
		"set_active":    setActive,
	}, nil
}

// TestSandboxConnection test sandbox connection
func (s *Service) TestSandboxConnection(providerType string, config map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{
		"provider_type": providerType,
		"config":        config,
		"connected":     true,
	}, nil
}

var heartBeatCount int64 = 0

// HandleHeartbeat handle heartbeat
func (s *Service) HandleHeartbeat(message *common.BaseMessage) (common.ErrorCode, string) {
	heartBeatCount++

	status := &common.BaseMessage{
		ServerName: message.ServerName,
		ServerType: message.ServerType,
		Host:       message.Host,
		Port:       message.Port,
		Version:    message.Version,
		Timestamp:  message.Timestamp,
		Ext:        message.Ext,
	}
	return UpdateServer(message.ServerName, status)
}

// InitDefaultAdmin initialize default admin user
// This matches Python's init_default_admin behavior
func (s *Service) InitDefaultAdmin() error {
	// Default superuser settings (matching Python's DEFAULT_SUPERUSER_* defaults)
	defaultNickname := "admin"
	defaultEmail := "admin@ragflow.io"
	defaultPassword := "admin"

	// Query superusers
	var users []*entity.User
	err := dao.DB.Where("is_superuser = ? AND status = ?", true, "1").Find(&users).Error
	if err != nil {
		return fmt.Errorf("failed to query superusers: %w", err)
	}

	if len(users) == 0 {
		userID := utility.GenerateToken()
		accessToken := utility.GenerateToken()
		status := "1"
		loginChannel := "password"
		isSuperuser := true

		// Python: password = encode_to_base64(password) = base64.b64encode(password)
		// Then: generate_password_hash(base64_password) creates werkzeug hash
		password := base64.StdEncoding.EncodeToString([]byte(defaultPassword))
		hashedPassword, err := common.GenerateWerkzeugPasswordHash(password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		user := &entity.User{
			ID:              userID,
			Email:           defaultEmail,
			Nickname:        defaultNickname,
			Password:        &hashedPassword,
			AccessToken:     &accessToken,
			Status:          &status,
			IsActive:        "1",
			IsAuthenticated: "1",
			IsAnonymous:     "0",
			LoginChannel:    &loginChannel,
			IsSuperuser:     &isSuperuser,
		}

		if err = dao.DB.Create(user).Error; err != nil {
			return fmt.Errorf("can't init admin: %w", err)
		}

		if err = s.addTenantForAdmin(userID, defaultNickname); err != nil {
			return fmt.Errorf("failed to add tenant for admin: %w", err)
		}

		common.Info("Init default super user successfully")
		return nil
	}

	for _, user := range users {
		if user.IsActive != "1" {
			return fmt.Errorf("no active admin. Please update 'is_active' in db manually")
		}
	}

	for _, user := range users {
		if user.Email == defaultEmail {
			// Check if tenant exists
			var count int64
			dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND status = ?", user.ID, "1").Count(&count)
			if count == 0 {
				nickname := defaultNickname
				if user.Nickname != "" {
					nickname = user.Nickname
				}
				if err = s.addTenantForAdmin(user.ID, nickname); err != nil {
					return err
				}
			}
			break
		}
	}

	common.Info("Init default super user successfully")
	return nil
}

// addTenantForAdmin add tenant for admin user
func (s *Service) addTenantForAdmin(userID, nickname string) error {
	status := "1"
	role := "owner"
	tenantName := nickname + "'s Kingdom"

	tenant := &entity.Tenant{
		ID:   userID,
		Name: &tenantName,
	}

	if err := dao.DB.Create(tenant).Error; err != nil {
		return err
	}

	userTenant := &entity.UserTenant{
		TenantID:  userID,
		UserID:    userID,
		InvitedBy: userID,
		Role:      role,
		Status:    &status,
	}

	return dao.DB.Create(userTenant).Error
}

// ListAllModels list all models
func (s *Service) ListAllModels(pageIndex, pageSize int) ([]map[string]interface{}, error) {
	models, err := dao.GetModelProviderManager().ListAllModels()
	if err != nil {
		return nil, err
	}
	if pageSize > 0 && pageIndex >= 0 && pageIndex*pageSize < len(models) {
		return models[pageIndex*pageSize : (pageIndex+1)*pageSize], nil
	}
	return models, nil
}

func (s *Service) GetModelByModelName(modelName string) (*modelModule.Model, error) {
	return dao.GetModelProviderManager().GetModelByNameOrAlias(modelName), nil
}
