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
	"ragflow/internal/config"
	"ragflow/internal/utility"
)

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

// VersionResponse version response
type VersionResponse struct {
	Version string `json:"version"`
}

// GetVersion get RAGFlow version
func (s *SystemService) GetVersion() (*VersionResponse, error) {
	version := utility.GetRAGFlowVersion()
	return &VersionResponse{
		Version: version,
	}, nil
}
