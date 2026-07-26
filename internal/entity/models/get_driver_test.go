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

package models

import (
	"strings"
	"testing"
)

func TestGetPreconfiguredDriverReturnsPreBuiltDriver(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "aliyun.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	driver, err := GetPreconfiguredDriver("Tongyi-Qianwen", "")
	if err != nil {
		t.Fatalf("GetPreconfiguredDriver: %v", err)
	}
	if driver == nil {
		t.Fatal("GetPreconfiguredDriver returned nil, want non-nil")
	}
	if _, ok := driver.(*AliyunModel); !ok {
		t.Fatalf("GetPreconfiguredDriver returned %T, want *AliyunModel", driver)
	}
}

func TestGetPreconfiguredDriverWithBaseURLOverride(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "aliyun.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	customURL := "https://custom-endpoint.example.com/v1"
	driver, err := GetPreconfiguredDriver("Tongyi-Qianwen", customURL)
	if err != nil {
		t.Fatalf("GetPreconfiguredDriver: %v", err)
	}
	if driver == nil {
		t.Fatal("GetPreconfiguredDriver returned nil, want non-nil")
	}

	// Verify the override took effect by checking GetBaseURL.
	aliModel, ok := driver.(*AliyunModel)
	if !ok {
		t.Fatalf("GetPreconfiguredDriver returned %T, want *AliyunModel", driver)
	}
	gotURL, err := aliModel.baseModel.GetBaseURL(&APIConfig{})
	if err != nil {
		t.Fatalf("GetBaseURL: %v", err)
	}
	if expected := strings.TrimSuffix(customURL, "/"); gotURL != expected {
		t.Errorf("GetBaseURL = %q, want %q", gotURL, expected)
	}
}

func TestGetPreconfiguredDriverProviderNotFound(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "aliyun.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	_, err := GetPreconfiguredDriver("NonExistentProvider", "")
	if err == nil {
		t.Fatal("GetPreconfiguredDriver should error for unknown provider, got nil")
	}
}

func TestGetPreconfiguredDriverManagerNotInitialized(t *testing.T) {
	// Save and reset the global so we can test the nil-manager path.
	saved := providerManager
	providerManager = nil
	defer func() { providerManager = saved }()

	_, err := GetPreconfiguredDriver("Tongyi-Qianwen", "")
	if err == nil {
		t.Fatal("GetPreconfiguredDriver should error when manager is nil, got nil")
	}
}

func TestGetPreconfiguredDriverSuffixTrimmed(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "aliyun.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	// Trailing slash should be stripped.
	driver, err := GetPreconfiguredDriver("Tongyi-Qianwen", "https://example.com/")
	if err != nil {
		t.Fatalf("GetPreconfiguredDriver: %v", err)
	}
	aliModel, ok := driver.(*AliyunModel)
	if !ok {
		t.Fatalf("expected *AliyunModel, got %T", driver)
	}
	gotURL, err := aliModel.baseModel.GetBaseURL(&APIConfig{})
	if err != nil {
		t.Fatalf("GetBaseURL: %v", err)
	}
	if gotURL != "https://example.com" {
		t.Errorf("GetBaseURL = %q, want %q", gotURL, "https://example.com")
	}
}

func TestGetPreconfiguredDriverDummyNoOverride(t *testing.T) {
	driver, err := GetPreconfiguredDriver("dummy", "")
	if err != nil {
		t.Fatalf("GetPreconfiguredDriver(dummy): %v", err)
	}
	if _, ok := driver.(*DummyModel); !ok {
		t.Fatalf("expected *DummyModel, got %T", driver)
	}
}

func TestGetPreconfiguredDriverDummyWithOverride(t *testing.T) {
	driver, err := GetPreconfiguredDriver("Dummy", "https://override.example.com")
	if err != nil {
		t.Fatalf("GetPreconfiguredDriver(Dummy): %v", err)
	}
	dummy, ok := driver.(*DummyModel)
	if !ok {
		t.Fatalf("expected *DummyModel, got %T", driver)
	}
	// The override should be stored in the driver's BaseURL.
	gotURL, err := dummy.baseModel.GetBaseURL(&APIConfig{})
	if err != nil {
		t.Fatalf("GetBaseURL: %v", err)
	}
	if gotURL != "https://override.example.com" {
		t.Errorf("GetBaseURL = %q, want %q", gotURL, "https://override.example.com")
	}
}
