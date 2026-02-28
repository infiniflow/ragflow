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
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/scrypt"

	"ragflow/internal/config"
	"ragflow/internal/dao"
	"ragflow/internal/model"
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
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
	Email    string `json:"email" binding:"required,email"`
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
func (s *UserService) Register(req *RegisterRequest) (*model.User, error) {
	// Check if email exists
	existUser, _ := s.userDAO.GetByEmail(req.Email)
	if existUser != nil {
		return nil, errors.New("email already exists")
	}

	// Generate password hash
	hashedPassword, err := s.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	status := "1"
	user := &model.User{
		Password: &hashedPassword,
		Email:    req.Email,
		Nickname: req.Nickname,
		Status:   &status,
	}

	if err := s.userDAO.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// Login user login
func (s *UserService) Login(req *LoginRequest) (*model.User, error) {
	// Get user by email (using username field as email)
	user, err := s.userDAO.GetByEmail(req.Username)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	// Decrypt password using RSA
	decryptedPassword, err := s.decryptPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt password: %w", err)
	}

	// Verify password
	if user.Password == nil || !s.VerifyPassword(*user.Password, decryptedPassword) {
		return nil, errors.New("invalid username or password")
	}

	// Check user status
	if user.Status == nil || *user.Status != "1" {
		return nil, errors.New("user is disabled")
	}

	// Generate new access token
	token := s.GenerateToken()
	if err := s.UpdateUserAccessToken(user, token); err != nil {
		return nil, fmt.Errorf("failed to update access token: %w", err)
	}

	// Update timestamp
	now := time.Now().Unix()
	user.UpdateTime = &now
	if err := s.userDAO.Update(user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

// LoginByEmail user login by email
func (s *UserService) LoginByEmail(req *EmailLoginRequest) (*model.User, error) {
	// Check for default admin account
	if req.Email == "admin@ragflow.io" {
		return nil, errors.New("default admin account cannot be used to login normal services")
	}

	// Get user by email
	user, err := s.userDAO.GetByEmail(req.Email)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	// Decrypt password using RSA
	decryptedPassword, err := s.decryptPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt password: %w", err)
	}

	// Verify password
	if user.Password == nil || !s.VerifyPassword(*user.Password, decryptedPassword) {
		return nil, errors.New("invalid email or password")
	}

	// Check user status
	if user.Status == nil || *user.Status != "1" {
		return nil, errors.New("user is disabled")
	}

	// Generate new access token
	token := s.GenerateToken()
	if err := s.UpdateUserAccessToken(user, token); err != nil {
		return nil, fmt.Errorf("failed to update access token: %w", err)
	}

	// Update timestamp
	now := time.Now().Unix()
	user.UpdateTime = &now
	if err := s.userDAO.Update(user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

// GetUserByID get user by ID
func (s *UserService) GetUserByID(id uint) (*UserResponse, error) {
	user, err := s.userDAO.GetByID(id)
	if err != nil {
		return nil, err
	}

	return &UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		Status:    user.Status,
		CreatedAt: time.Unix(user.CreateTime, 0).Format("2006-01-02 15:04:05"),
	}, nil
}

// ListUsers list users
func (s *UserService) ListUsers(page, pageSize int) ([]*UserResponse, int64, error) {
	offset := (page - 1) * pageSize
	users, total, err := s.userDAO.List(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*UserResponse, len(users))
	for i, user := range users {
		responses[i] = &UserResponse{
			ID:        user.ID,
			Email:     user.Email,
			Nickname:  user.Nickname,
			Status:    user.Status,
			CreatedAt: time.Unix(user.CreateTime, 0).Format("2006-01-02 15:04:05"),
		}
	}

	return responses, total, nil
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
func (s *UserService) VerifyPassword(hashedPassword, password string) bool {
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

// GenerateToken generates a new access token
func (s *UserService) GenerateToken() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// GetUserByToken gets user by access token
func (s *UserService) GetUserByToken(token string) (*model.User, error) {
	return s.userDAO.GetByAccessToken(token)
}

// UpdateUserAccessToken updates user's access token
func (s *UserService) UpdateUserAccessToken(user *model.User, token string) error {
	return s.userDAO.UpdateAccessToken(user, token)
}

// Logout invalidates user's access token
func (s *UserService) Logout(user *model.User) error {
	// Invalidate token by setting it to an invalid value
	// Similar to Python implementation: "INVALID_" + secrets.token_hex(16)
	invalidToken := "INVALID_" + s.GenerateToken()
	return s.UpdateUserAccessToken(user, invalidToken)
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
func (s *UserService) UpdateUserSettings(user *model.User, req *UpdateSettingsRequest) error {
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
	return s.userDAO.Update(user)
}

// ChangePassword changes user password
func (s *UserService) ChangePassword(user *model.User, req *ChangePasswordRequest) error {
	// If password is provided, verify current password
	if req.Password != nil {
		if user.Password == nil || !s.VerifyPassword(*user.Password, *req.Password) {
			return errors.New("current password is incorrect")
		}
	}

	// If new password is provided, update password
	if req.NewPassword != nil {
		hashedPassword, err := s.HashPassword(*req.NewPassword)
		if err != nil {
			return fmt.Errorf("failed to hash new password: %w", err)
		}
		user.Password = &hashedPassword
	}

	// Save updated user
	return s.userDAO.Update(user)
}

// LoginChannel represents a login channel response
type LoginChannel struct {
	Channel     string `json:"channel"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
}

// GetLoginChannels gets all supported authentication channels
func (s *UserService) GetLoginChannels() ([]*LoginChannel, error) {
	cfg := config.Get()
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

	return channels, nil
}
