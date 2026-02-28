package service

import "ragflow/internal/config"

// SystemService system service
type SystemService struct{}

// NewSystemService create system service
func NewSystemService() *SystemService {
	return &SystemService{}
}

// ConfigResponse system configuration response
type ConfigResponse struct {
	RegisterEnabled int `json:"registerEnabled"`
}

// GetConfig get system configuration
func (s *SystemService) GetConfig() (*ConfigResponse, error) {
	cfg := config.Get()
	return &ConfigResponse{
		RegisterEnabled: cfg.RegisterEnabled,
	}, nil
}
