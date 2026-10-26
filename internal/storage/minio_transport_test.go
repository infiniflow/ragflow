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

package storage

import (
	"crypto/tls"
	"ragflow/internal/server/config"
	"testing"
	"time"
)

func TestNewMinioTransport_SecureWithVerify(t *testing.T) {
	cfg := config.MinioConfig{
		Secure: true,
		Verify: true,
	}

	transport, err := newMinioTransport(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating minio transport: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}

	if transport.MaxIdleConnsPerHost != 16 {
		t.Errorf("expected MaxIdleConnsPerHost to be 16, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxIdleConns != 256 {
		t.Errorf("expected MaxIdleConns to be 256, got %d", transport.MaxIdleConns)
	}
	if !transport.DisableCompression {
		t.Error("expected DisableCompression to be true")
	}
	if transport.IdleConnTimeout != time.Minute {
		t.Errorf("expected IdleConnTimeout to be 1m, got %v", transport.IdleConnTimeout)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("expected non-nil TLSClientConfig when Secure is true")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be false when Verify is true")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected TLS MinVersion to be TLS 1.2, got %x", transport.TLSClientConfig.MinVersion)
	}
}

func TestNewMinioTransport_SecureSkipVerify(t *testing.T) {
	cfg := config.MinioConfig{
		Secure: true,
		Verify: false,
	}

	transport, err := newMinioTransport(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating minio transport: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}

	if transport.MaxIdleConnsPerHost != 16 {
		t.Errorf("expected MaxIdleConnsPerHost to be 16, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("expected non-nil TLSClientConfig when Secure is true")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true when Verify is false")
	}
}

func TestNewMinioTransport_Insecure(t *testing.T) {
	cfg := config.MinioConfig{
		Secure: false,
	}

	transport, err := newMinioTransport(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating minio transport: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}

	if transport.MaxIdleConnsPerHost != 16 {
		t.Errorf("expected MaxIdleConnsPerHost to be 16, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxIdleConns != 256 {
		t.Errorf("expected MaxIdleConns to be 256, got %d", transport.MaxIdleConns)
	}
	if !transport.DisableCompression {
		t.Error("expected DisableCompression to be true")
	}
}

func TestNewMinioStorage_InitializesClientWithTransport(t *testing.T) {
	cfg := config.MinioConfig{
		Host:     "localhost:9000",
		User:     "minioadmin",
		Password: "minioadmin",
		Secure:   true,
		Verify:   false,
	}

	storage, err := NewMinioStorage(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating MinioStorage: %v", err)
	}
	if storage == nil || storage.client == nil {
		t.Fatal("expected non-nil storage and client")
	}
}
