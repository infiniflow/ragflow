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

package service

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"

	"ragflow/internal/dao"
	"ragflow/internal/model"
	"ragflow/internal/utility"
)

// UserService user service
type UserService struct {
	userDAO *dao.UserDAO
}

// NewUserService create user service
func NewUserService() *UserService {
	return &UserService{
		userDAO: dao.NewUserDAO(),
	}
}

// RegisterRequest registration request
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Nickname string `json:"nickname"`
}

// LoginRequest login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// EmailLoginRequest email login request
type EmailLoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// UpdateSettingsRequest update user settings request
type UpdateSettingsRequest struct {
	Nickname    *string `json:"nickname,omitempty"`
	Email       *string `json:"email,omitempty" binding:"omitempty,email"`
	Avatar      *string `json:"avatar,omitempty"`
	Language    *string `json:"language,omitempty"`
	ColorSchema *string `json:"color_schema,omitempty"`
	Timezone    *string `json:"timezone,omitempty"`
}

// ChangePasswordRequest change password request
type ChangePasswordRequest struct {
	Password    *string `json:"password,omitempty"`
	NewPassword *string `json:"new_password,omitempty"`
}

// UserResponse user response
type UserResponse struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Nickname  string  `json:"nickname"`
	Status    *string `json:"status"`
	CreatedAt string  `json:"created_at"`
}

// Register user registration
func (s *UserService) Register(req *RegisterRequest) (*model.User, common.ErrorCode, error) {
	cfg := server.GetConfig()
	if cfg.RegisterEnabled == 0 {
		return nil, common.CodeOperatingError, fmt.Errorf("User registration is disabled!")
	}

	emailRegex := regexp.MustCompile(`^[\w\._-]+@([\w_-]+\.)+[\w-]{2,}$`)
	if !emailRegex.MatchString(req.Email) {
		return nil, common.CodeOperatingError, fmt.Errorf("Invalid email address: %s!", req.Email)
	}

	existUser, _ := s.userDAO.GetByEmail(req.Email)
	if existUser != nil {
		return nil, common.CodeOperatingError, fmt.Errorf("Email: %s has already registered!", req.Email)
	}

	decryptedPassword, err := s.decryptPassword(req.Password)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("Fail to decrypt password")
	}

	hashedPassword, err := s.HashPassword(decryptedPassword)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to hash password: %w", err)
	}

	userID := utility.GenerateToken()
	accessToken := utility.GenerateToken()
	status := "1"
	loginChannel := "password"
	isSuperuser := false

	user := &model.User{
		ID:              userID,
		AccessToken:     &accessToken,
		Email:           req.Email,
		Nickname:        req.Nickname,
		Password:        &hashedPassword,
		Status:          &status,
		IsActive:        "1",
		IsAuthenticated: "1",
		IsAnonymous:     "0",
		LoginChannel:    &loginChannel,
		IsSuperuser:     &isSuperuser,
	}

	now := time.Now().Unix()
	user.CreateTime = &now
	user.UpdateTime = &now
	now_date := time.Now()
	user.CreateDate = &now_date
	user.UpdateDate = &now_date
	user.LastLoginTime = &now_date

	tenantName := req.Nickname + "'s Kingdom"

	llmID := cfg.UserDefaultLLM.DefaultModels.ChatModel.Name
	if llmID == "" {
		llmID = ""
	}
	embdID := cfg.UserDefaultLLM.DefaultModels.EmbeddingModel.Name
	if embdID == "" {
		embdID = ""
	}
	asrID := cfg.UserDefaultLLM.DefaultModels.ASRModel.Name
	if asrID == "" {
		asrID = ""
	}
	img2txtID := cfg.UserDefaultLLM.DefaultModels.Image2TextModel.Name
	if img2txtID == "" {
		img2txtID = ""
	}
	rerankID := cfg.UserDefaultLLM.DefaultModels.RerankModel.Name
	if rerankID == "" {
		rerankID = ""
	}

	tenant := &model.Tenant{
		ID:        userID,
		Name:      &tenantName,
		LLMID:     llmID,
		EmbdID:    embdID,
		ASRID:     asrID,
		Img2TxtID: img2txtID,
		RerankID:  rerankID,
		ParserIDs: "naive:General,Q&A:Q&A,manual:Manual,table:Table,paper:Research Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,email:Email,tag:Tag",
		Status:    &status,
	}
	tenant.CreateTime = &now
	tenant.UpdateTime = &now
	tenant.CreateDate = &now_date
	tenant.UpdateDate = &now_date

	userTenantID := utility.GenerateToken()
	userTenant := &model.UserTenant{
		ID:        userTenantID,
		UserID:    userID,
		TenantID:  userID,
		Role:      "owner",
		InvitedBy: userID,
		Status:    &status,
	}
	userTenant.CreateTime = &now
	userTenant.UpdateTime = &now
	userTenant.CreateDate = &now_date
	userTenant.UpdateDate = &now_date

	fileID := utility.GenerateToken()
	rootFile := &model.File{
		ID:        fileID,
		ParentID:  fileID,
		TenantID:  userID,
		CreatedBy: userID,
		Name:      "/",
		Type:      "folder",
		Size:      0,
	}
	rootFile.CreateTime = &now
	rootFile.UpdateTime = &now
	rootFile.CreateDate = &now_date
	rootFile.UpdateDate = &now_date

	tenantDAO := dao.NewTenantDAO()
	userTenantDAO := dao.NewUserTenantDAO()
	fileDAO := dao.NewFileDAO()

	if err := s.userDAO.Create(user); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to create user: %w", err)
	}

	if err := tenantDAO.Create(tenant); err != nil {
		err := s.userDAO.DeleteByID(userID)
		if err != nil {
			return nil, 0, err
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to create tenant: %w", err)
	}

	if err := userTenantDAO.Create(userTenant); err != nil {
		err := s.userDAO.DeleteByID(userID)
		if err != nil {
			return nil, 0, err
		}
		err = tenantDAO.Delete(userID)
		if err != nil {
			return nil, 0, err
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to create user tenant relation: %w", err)
	}

	if err := fileDAO.Create(rootFile); err != nil {
		err := s.userDAO.DeleteByID(userID)
		if err != nil {
			return nil, 0, err
		}
		err = tenantDAO.Delete(userID)
		if err != nil {
			return nil, 0, err
		}
		err = userTenantDAO.Delete(userTenantID)
		if err != nil {
			return nil, 0, err
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to create root folder: %w", err)
	}

	return user, common.CodeSuccess, nil
}

// Login user login
func (s *UserService) Login(req *LoginRequest) (*model.User, common.ErrorCode, error) {
	// Get user by email (using username field as email)
	user, err := s.userDAO.GetByEmail(req.Username)
	if err != nil {
		return nil, common.CodeAuthenticationError, fmt.Errorf("invalid email or password")
	}

	// Decrypt password using RSA
	decryptedPassword, err := s.decryptPassword(req.Password)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to decrypt password: %w", err)
	}

	// Verify password
	if user.Password == nil || !s.VerifyPassword(*user.Password, decryptedPassword) {
		return nil, common.CodeAuthenticationError, fmt.Errorf("invalid username or password")
	}

	if user.Status == nil || *user.Status != "1" {
		return nil, common.CodeForbidden, fmt.Errorf("user is disabled")
	}

	// Generate new access token
	token := utility.GenerateToken()
	if err := s.UpdateUserAccessToken(user, token); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to update access token: %w", err)
	}

	// Update timestamp
	now := time.Now().Unix()
	user.UpdateTime = &now
	if err := s.userDAO.Update(user); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to update user: %w", err)
	}

	return user, common.CodeSuccess, nil
}

// LoginByEmail user login by email
// Returns user on success, or error with specific code:
// - CodeAuthenticationError (109): Email not registered or password mismatch
// - CodeServerError (500): Password decryption failure
// - CodeForbidden (403): Account disabled
func (s *UserService) LoginByEmail(req *EmailLoginRequest, adminLogin bool) (*model.User, common.ErrorCode, error) {
	if !adminLogin && req.Email == "admin@ragflow.io" {
		return nil, common.CodeAuthenticationError, fmt.Errorf("default admin account cannot be used to login normal services")
	}

	user, err := s.userDAO.GetByEmail(req.Email)
	if err != nil {
		return nil, common.CodeAuthenticationError, fmt.Errorf("Email: %s is not registered!", req.Email)
	}

	decryptedPassword, err := s.decryptPassword(req.Password)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("Fail to crypt password")
	}

	if user.Password == nil || !s.VerifyPassword(*user.Password, decryptedPassword) {
		return nil, common.CodeAuthenticationError, fmt.Errorf("Email and password do not match!")
	}

	if user.IsActive == "0" {
		return nil, common.CodeForbidden, fmt.Errorf("This account has been disabled, please contact the administrator!")
	}

	// Generate new access token
	token := utility.GenerateToken()
	user.AccessToken = &token

	now := time.Now().Unix()
	user.UpdateTime = &now
	now_date := time.Now()
	user.UpdateDate = &now_date
	if err := s.userDAO.Update(user); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to update user: %w", err)
	}

	return user, common.CodeSuccess, nil
}

// GetUserByID get user by ID
func (s *UserService) GetUserByID(id uint) (*UserResponse, common.ErrorCode, error) {
	user, err := s.userDAO.GetByID(id)
	if err != nil {
		return nil, common.CodeNotFound, err
	}

	return &UserResponse{
		ID:       user.ID,
		Email:    user.Email,
		Nickname: user.Nickname,
		Status:   user.Status,
		CreatedAt: func() string {
			if user.CreateTime != nil {
				return time.Unix(*user.CreateTime, 0).Format("2006-01-02 15:04:05")
			}
			return ""
		}(),
	}, common.CodeSuccess, nil
}

// ListUsers list users
func (s *UserService) ListUsers(page, pageSize int) ([]*UserResponse, int64, common.ErrorCode, error) {
	offset := (page - 1) * pageSize
	users, total, err := s.userDAO.List(offset, pageSize)
	if err != nil {
		return nil, 0, common.CodeServerError, err
	}

	responses := make([]*UserResponse, len(users))
	for i, user := range users {
		responses[i] = &UserResponse{
			ID:       user.ID,
			Email:    user.Email,
			Nickname: user.Nickname,
			Status:   user.Status,
			CreatedAt: func() string {
				if user.CreateTime != nil {
					return time.Unix(*user.CreateTime, 0).Format("2006-01-02 15:04:05")
				}
				return ""
			}(),
		}
	}

	return responses, total, common.CodeSuccess, nil
}

// HashPassword generate password hash
func (s *UserService) HashPassword(password string) (string, error) {
	salt := s.generateSalt()
	hash, err := scrypt.Key([]byte(password), salt, 32768, 8, 1, 64)
	if err != nil {
		return "", err
	}

	// Return werkzeug format: scrypt:n:r:p$salt$hash
	return fmt.Sprintf("scrypt:32768:8:1$%s$%x", string(salt), hash), nil
}

// VerifyPassword verify password
// Supports both werkzeug pbkdf2 format (pbkdf2:sha256:iterations$salt$hash) and scrypt format
func (s *UserService) VerifyPassword(hashedPassword, password string) bool {
	// Check if it's pbkdf2 format (werkzeug)
	if strings.HasPrefix(hashedPassword, "pbkdf2:") {
		return s.verifyPBKDF2Password(hashedPassword, password)
	}

	// Check if it's scrypt format
	if strings.HasPrefix(hashedPassword, "scrypt:") {
		return s.verifyScryptPassword(hashedPassword, password)
	}

	return false
}

// verifyPBKDF2Password verifies password using PBKDF2 (werkzeug format)
// Format: pbkdf2:sha256:iterations$salt$hash
func (s *UserService) verifyPBKDF2Password(hashedPassword, password string) bool {
	parts := strings.Split(hashedPassword, "$")
	if len(parts) != 3 {
		return false
	}

	// Parse method (e.g., "pbkdf2:sha256:150000")
	methodParts := strings.Split(parts[0], ":")
	if len(methodParts) != 3 {
		return false
	}

	if methodParts[0] != "pbkdf2" {
		return false
	}

	var hashFunc func() hash.Hash
	switch methodParts[1] {
	case "sha256":
		hashFunc = sha256.New
	case "sha512":
		hashFunc = sha512.New
	default:
		return false
	}

	iterations, err := strconv.Atoi(methodParts[2])
	if err != nil {
		return false
	}

	salt := parts[1]
	expectedHash := parts[2]

	// Decode salt from base64
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		// Try hex encoding
		saltBytes, err = hex.DecodeString(salt)
		if err != nil {
			return false
		}
	}

	// Generate hash using PBKDF2
	key := pbkdf2.Key([]byte(password), saltBytes, iterations, 32, hashFunc)
	computedHash := base64.StdEncoding.EncodeToString(key)

	return computedHash == expectedHash
}

// verifyScryptPassword verifies password using scrypt format
// Format: scrypt:n:r:p$salt$hash
func (s *UserService) verifyScryptPassword(hashedPassword, password string) bool {
	// Parse hash format: scrypt:n:r:p$salt$hash
	parts := strings.Split(hashedPassword, "$")
	if len(parts) != 3 {
		return false
	}

	params := strings.Split(parts[0], ":")
	if len(params) != 4 || params[0] != "scrypt" {
		return false
	}

	n, err := strconv.ParseUint(params[1], 10, 0)
	if err != nil {
		return false
	}
	r, err := strconv.ParseUint(params[2], 10, 0)
	if err != nil {
		return false
	}
	p, err := strconv.ParseUint(params[3], 10, 0)
	if err != nil {
		return false
	}

	saltStr := parts[1]
	hashHex := parts[2]

	// Compute password hash
	computed, err := scrypt.Key([]byte(password), []byte(saltStr), int(n), int(r), int(p), len(hashHex)/2)
	if err != nil {
		return false
	}

	decodedHash, err := hex.DecodeString(hashHex)

	// Constant time comparison
	return s.constantTimeCompare(decodedHash, computed)
}

// generateSalt generate salt
func (s *UserService) generateSalt() []byte {
	return []byte("random_salt_for_user") // TODO: use random salt
}

// constantTimeCompare constant time comparison
func (s *UserService) constantTimeCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}

	return result == 0
}

// loadPrivateKey loads and decrypts the RSA private key from conf/private.pem
// nolint:staticcheck // DecryptPEMBlock is deprecated but still works for traditional PEM encryption
func (s *UserService) loadPrivateKey() (*rsa.PrivateKey, error) {
	// Read private key file
	keyData, err := os.ReadFile("conf/private.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	// Decrypt the PEM block if it's encrypted
	var privateKey interface{}
	if block.Headers["Proc-Type"] == "4,ENCRYPTED" {
		// Decrypt using password "Welcome"
		// Note: DecryptPEMBlock is deprecated but still functional for traditional PEM encryption
		decryptedData, err := x509.DecryptPEMBlock(block, []byte("Welcome"))
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt private key: %w", err)
		}

		// Parse the decrypted key
		privateKey, err = x509.ParsePKCS1PrivateKey(decryptedData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	} else {
		// Not encrypted, parse directly
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	}

	rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an RSA private key")
	}

	return rsaPrivateKey, nil
}

// decryptPassword decrypts the password using RSA private key
func (s *UserService) decryptPassword(encryptedPassword string) (string, error) {
	// Try to decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedPassword)
	if err != nil {
		// If base64 decoding fails, assume it's already a plain password
		return encryptedPassword, nil
	}

	// Load private key
	privateKey, err := s.loadPrivateKey()
	if err != nil {
		return "", err
	}

	// Decrypt using PKCS#1 v1.5
	plaintext, err := rsa.DecryptPKCS1v15(nil, privateKey, ciphertext)
	if err != nil {
		// If decryption fails, assume it's already a plain password
		return encryptedPassword, nil
	}

	return string(plaintext), nil
}

// GetUserByToken gets user by authorization header
// The token parameter is the authorization header value, which needs to be decrypted
// using itsdangerous URLSafeTimedSerializer to get the actual access_token
func (s *UserService) GetUserByToken(authorization string) (*model.User, common.ErrorCode, error) {
	// Get secret key from config
	variables := server.GetVariables()
	secretKey := variables.SecretKey

	// Extract access token from authorization header
	// Equivalent to: access_token = str(jwt.loads(authorization)) in Python
	accessToken, err := utility.ExtractAccessToken(authorization, secretKey)
	if err != nil {
		return nil, common.CodeUnauthorized, fmt.Errorf("invalid authorization token: %w", err)
	}

	// Validate token format (should be at least 32 chars, UUID format)
	if len(accessToken) < 32 {
		return nil, common.CodeUnauthorized, fmt.Errorf("invalid access token format")
	}

	// Get user by access token
	user, err := s.userDAO.GetByAccessToken(accessToken)
	if err != nil {
		return nil, common.CodeUnauthorized, err
	}

	return user, common.CodeSuccess, nil
}

// UpdateUserAccessToken updates user's access token
func (s *UserService) UpdateUserAccessToken(user *model.User, token string) error {
	return s.userDAO.UpdateAccessToken(user, token)
}

// Logout invalidates user's access token
func (s *UserService) Logout(user *model.User) (common.ErrorCode, error) {
	// Invalidate token by setting it to an invalid value
	// Similar to Python implementation: "INVALID_" + secrets.token_hex(16)
	invalidToken := "INVALID_" + utility.GenerateToken()
	err := s.UpdateUserAccessToken(user, invalidToken)
	if err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

// GetUserProfile returns user profile information
func (s *UserService) GetUserProfile(user *model.User) map[string]interface{} {
	// Format create time and date (from database fields)
	createTime := user.CreateTime
	createDate := ""
	if user.CreateDate != nil {
		createDate = user.CreateDate.Format("2006-01-02T15:04:05")
	}

	// Format update time and date (from database fields)
	var updateTime int64
	updateDate := ""
	if user.UpdateTime != nil {
		updateTime = *user.UpdateTime
	}
	if user.UpdateDate != nil {
		updateDate = user.UpdateDate.Format("2006-01-02T15:04:05")
	}

	// Format last login time
	var lastLoginTime string
	if user.LastLoginTime != nil {
		lastLoginTime = user.LastLoginTime.Format("2006-01-02T15:04:05")
	}

	// Get access token
	var accessToken string
	if user.AccessToken != nil {
		accessToken = *user.AccessToken
	}

	// Get avatar
	var avatar interface{}
	if user.Avatar != nil {
		avatar = *user.Avatar
	} else {
		avatar = nil
	}

	// Get color schema
	colorSchema := "Bright"
	if user.ColorSchema != nil && *user.ColorSchema != "" {
		colorSchema = *user.ColorSchema
	}

	// Get language
	language := "English"
	if user.Language != nil && *user.Language != "" {
		language = *user.Language
	}

	// Get timezone
	timezone := "UTC+8\tAsia/Shanghai"
	if user.Timezone != nil && *user.Timezone != "" {
		timezone = *user.Timezone
	}

	// Get login channel
	loginChannel := "password"
	if user.LoginChannel != nil && *user.LoginChannel != "" {
		loginChannel = *user.LoginChannel
	}

	// Get password
	var password string
	if user.Password != nil {
		password = *user.Password
	}

	// Get status
	status := "1"
	if user.Status != nil {
		status = *user.Status
	}

	// Get is_superuser
	isSuperuser := false
	if user.IsSuperuser != nil {
		isSuperuser = *user.IsSuperuser
	}

	return map[string]interface{}{
		"access_token":     accessToken,
		"avatar":           avatar,
		"color_schema":     colorSchema,
		"create_date":      createDate,
		"create_time":      createTime,
		"email":            user.Email,
		"id":               user.ID,
		"is_active":        user.IsActive,
		"is_anonymous":     user.IsAnonymous,
		"is_authenticated": user.IsAuthenticated,
		"is_superuser":     isSuperuser,
		"language":         language,
		"last_login_time":  lastLoginTime,
		"login_channel":    loginChannel,
		"nickname":         user.Nickname,
		"password":         password,
		"status":           status,
		"timezone":         timezone,
		"update_date":      updateDate,
		"update_time":      updateTime,
	}
}

// UpdateUserSettings updates user settings
func (s *UserService) UpdateUserSettings(user *model.User, req *UpdateSettingsRequest) (common.ErrorCode, error) {
	// Update fields if provided
	if req.Nickname != nil {
		user.Nickname = *req.Nickname
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Avatar != nil {
		// In Go version, avatar might be stored differently
		// For now, just update if field exists
	}
	if req.Language != nil {
		// Store language preference
	}
	if req.ColorSchema != nil {
		// Store color schema preference
	}
	if req.Timezone != nil {
		// Store timezone preference
	}

	// Save updated user
	if err := s.userDAO.Update(user); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

// ChangePassword changes user password
func (s *UserService) ChangePassword(user *model.User, req *ChangePasswordRequest) (common.ErrorCode, error) {
	// If password is provided, verify current password
	if req.Password != nil {
		if user.Password == nil || !s.VerifyPassword(*user.Password, *req.Password) {
			return common.CodeBadRequest, fmt.Errorf("current password is incorrect")
		}
	}

	// If new password is provided, update password
	if req.NewPassword != nil {
		hashedPassword, err := s.HashPassword(*req.NewPassword)
		if err != nil {
			return common.CodeServerError, fmt.Errorf("failed to hash new password: %w", err)
		}
		user.Password = &hashedPassword
	}

	// Save updated user
	if err := s.userDAO.Update(user); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

// LoginChannel represents a login channel response
type LoginChannel struct {
	Channel     string `json:"channel"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
}

// GetLoginChannels gets all supported authentication channels
func (s *UserService) GetLoginChannels() ([]*LoginChannel, common.ErrorCode, error) {
	cfg := server.GetConfig()
	channels := make([]*LoginChannel, 0)

	for channel, oauthCfg := range cfg.OAuth {
		displayName := oauthCfg.DisplayName
		if displayName == "" {
			displayName = strings.Title(channel)
		}

		icon := oauthCfg.Icon
		if icon == "" {
			icon = "sso"
		}

		channels = append(channels, &LoginChannel{
			Channel:     channel,
			DisplayName: displayName,
			Icon:        icon,
		})
	}

	return channels, common.CodeSuccess, nil
}

// SetTenantInfoRequest represents the request for setting tenant info
type SetTenantInfoRequest struct {
	TenantID  string `json:"tenant_id"`
	ASRID     string `json:"asr_id"`
	EmbdID    string `json:"embd_id"`
	Img2TxtID string `json:"img2txt_id"`
	LLMID     string `json:"llm_id"`
	RerankID  string `json:"rerank_id"`
	TTSID     string `json:"tts_id"`
}

// SetTenantInfo updates tenant model configuration
func (s *UserService) SetTenantInfo(userID string, req *SetTenantInfoRequest) error {
	tenantDAO := dao.NewTenantDAO()

	_, err := tenantDAO.GetByID(req.TenantID)
	if err != nil {
		return fmt.Errorf("tenant not found: %w", err)
	}

	updates := make(map[string]interface{})
	if req.LLMID != "" {
		updates["llm_id"] = req.LLMID
	}
	if req.EmbdID != "" {
		updates["embd_id"] = req.EmbdID
	}
	if req.ASRID != "" {
		updates["asr_id"] = req.ASRID
	}
	if req.Img2TxtID != "" {
		updates["img2txt_id"] = req.Img2TxtID
	}
	if req.RerankID != "" {
		updates["rerank_id"] = req.RerankID
	}
	if req.TTSID != "" {
		updates["tts_id"] = req.TTSID
	}

	if len(updates) > 0 {
		if err := tenantDAO.Update(req.TenantID, updates); err != nil {
			return fmt.Errorf("failed to update tenant: %w", err)
		}
	}

	return nil
}
