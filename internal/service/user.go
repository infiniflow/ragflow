package service

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/scrypt"

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
	ID        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}

// Register user registration
func (s *UserService) Register(req *RegisterRequest) (*model.User, error) {
	// Check if username exists
	existUser, _ := s.userDAO.GetByUsername(req.Username)
	if existUser != nil {
		return nil, errors.New("username already exists")
	}

	// Generate password hash
	hashedPassword, err := s.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &model.User{
		Username: req.Username,
		Password: &hashedPassword,
		Email:    req.Email,
		Nickname: req.Nickname,
		Status:   1,
	}

	if err := s.userDAO.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// Login user login
func (s *UserService) Login(req *LoginRequest) (*model.User, error) {
	// Get user
	user, err := s.userDAO.GetByUsername(req.Username)
	if err != nil {
		return nil, errors.New("invalid username or password")
	}

	// Verify password
	if user.Password == nil || !s.VerifyPassword(*user.Password, req.Password) {
		return nil, errors.New("invalid username or password")
	}

	// Check user status
	if user.Status != 1 {
		return nil, errors.New("user is disabled")
	}

	// Generate new access token
	token := s.GenerateToken()
	if err := s.UpdateUserAccessToken(user, token); err != nil {
		return nil, fmt.Errorf("failed to update access token: %w", err)
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

	// Verify password
	if user.Password == nil || !s.VerifyPassword(*user.Password, req.Password) {
		return nil, errors.New("invalid email or password")
	}

	// Check user status
	if user.Status != 1 {
		return nil, errors.New("user is disabled")
	}

	// Generate new access token
	token := s.GenerateToken()
	if err := s.UpdateUserAccessToken(user, token); err != nil {
		return nil, fmt.Errorf("failed to update access token: %w", err)
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
		Username:  user.Username,
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
			Username:  user.Username,
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
	updatedAt := ""
	if user.UpdateTime != nil {
		updatedAt = time.Unix(*user.UpdateTime, 0).Format("2006-01-02 15:04:05")
	}
	return map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"nickname":   user.Nickname,
		"status":     user.Status,
		"created_at": time.Unix(user.CreateTime, 0).Format("2006-01-02 15:04:05"),
		"updated_at": updatedAt,
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
