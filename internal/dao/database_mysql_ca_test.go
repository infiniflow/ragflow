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

package dao

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ragflow/internal/server/config"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func mysqlCATestConfig() config.MySQLConfig {
	return config.MySQLConfig{
		DatabaseName: "rag_flow",
		User:         "root",
		Password:     "secret",
		Host:         "localhost",
		Port:         3306,
		Charset:      "utf8mb4",
	}
}

func TestBuildMySQLMetaDSNCustomTLSRequiresCAFile(t *testing.T) {
	cfg := mysqlCATestConfig()
	cfg.TLS = "custom"
	if _, err := buildMySQLMetaDSN(cfg); err == nil {
		t.Error("expected an error when custom tls has no CA file")
	}
}

func TestBuildMySQLMetaDSNCustomTLSRegistersCA(t *testing.T) {
	cfg := mysqlCATestConfig()
	cfg.TLS = "custom"
	cfg.TLSCAFile = writeTempCAFile(t)

	dsn, err := buildMySQLMetaDSN(cfg)
	if err != nil {
		t.Fatalf("buildMySQLMetaDSN: %v", err)
	}
	if !strings.Contains(dsn, "tls="+customMySQLTLSConfig) {
		t.Fatalf("DSN = %q, want tls=%s", dsn, customMySQLTLSConfig)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if parsed.TLSConfig != customMySQLTLSConfig {
		t.Fatalf("tls config = %q, want %q", parsed.TLSConfig, customMySQLTLSConfig)
	}
	// ParseDSN resolves a named TLS config through the driver's registry into
	// Config.TLS, proving the CA-backed config was registered.
	if parsed.TLS == nil || parsed.TLS.RootCAs == nil || len(parsed.TLS.RootCAs.Subjects()) == 0 {
		t.Fatal("custom tls config was not registered with a CA pool")
	}
	if parsed.TLS.ServerName != cfg.Host {
		t.Errorf("ServerName = %q, want %q", parsed.TLS.ServerName, cfg.Host)
	}
}

func TestBuildMySQLMetaDSNCustomTLSMissingCAFile(t *testing.T) {
	cfg := mysqlCATestConfig()
	cfg.TLS = "custom"
	cfg.TLSCAFile = filepath.Join(t.TempDir(), "missing.pem")
	if _, err := buildMySQLMetaDSN(cfg); err == nil {
		t.Error("expected an error for a missing CA file")
	}
}

func TestBuildMySQLMetaDSNCustomTLSRejectsInvalidPEM(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := mysqlCATestConfig()
	cfg.TLS = "custom"
	cfg.TLSCAFile = caFile
	if _, err := buildMySQLMetaDSN(cfg); err == nil {
		t.Error("expected an error for an invalid PEM CA file")
	}
}

func writeTempCAFile(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ragflow-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(caFile, pemBytes, 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	return caFile
}
