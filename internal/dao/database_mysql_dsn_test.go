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
	"testing"
	"time"

	"ragflow/internal/server/config"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func baseMySQLConfig() config.MySQLConfig {
	return config.MySQLConfig{
		DatabaseName: "rag_flow",
		User:         "root",
		Password:     "infini_rag_flow",
		Host:         "localhost",
		Port:         3306,
		Charset:      "utf8mb4",
	}
}

func TestBuildMySQLDSNDefaultsMatchLegacyConnection(t *testing.T) {
	dsn, err := buildMySQLDSN(baseMySQLConfig())
	if err != nil {
		t.Fatalf("buildMySQLDSN: %v", err)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn %q: %v", dsn, err)
	}
	if parsed.User != "root" || parsed.Passwd != "infini_rag_flow" {
		t.Errorf("credentials = %s/%s", parsed.User, parsed.Passwd)
	}
	if parsed.Net != "tcp" || parsed.Addr != "localhost:3306" {
		t.Errorf("connection target = %s %s", parsed.Net, parsed.Addr)
	}
	if parsed.DBName != "rag_flow" {
		t.Errorf("db name = %s", parsed.DBName)
	}
	if !parsed.ParseTime {
		t.Error("ParseTime should be enabled")
	}
	if parsed.Loc != time.Local {
		t.Errorf("loc = %v, want Local", parsed.Loc)
	}
	if parsed.Params["charset"] != "utf8mb4" {
		t.Errorf("charset = %q", parsed.Params["charset"])
	}
	if parsed.TLSConfig != "" {
		t.Errorf("tls config = %q, want empty for the default plaintext connection", parsed.TLSConfig)
	}
	// These defaults come from the driver, not from the DSN string; building
	// the Config as a bare struct literal silently turns them off.
	if !parsed.AllowNativePasswords {
		t.Error("AllowNativePasswords should stay enabled")
	}
	if !parsed.CheckConnLiveness {
		t.Error("CheckConnLiveness should stay enabled")
	}
	if parsed.MaxAllowedPacket != mysqldriver.NewConfig().MaxAllowedPacket {
		t.Errorf("MaxAllowedPacket = %d, want driver default", parsed.MaxAllowedPacket)
	}
}

func TestBuildMySQLDSNSpecialPasswordRoundTrip(t *testing.T) {
	cfg := baseMySQLConfig()
	cfg.Password = `p@ss:w/d?#[]%`
	dsn, err := buildMySQLDSN(cfg)
	if err != nil {
		t.Fatalf("buildMySQLDSN: %v", err)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if parsed.Passwd != cfg.Password {
		t.Errorf("password round trip = %q, want %q", parsed.Passwd, cfg.Password)
	}
}

func TestBuildMySQLDSNTLSModes(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"false":       "",
		"true":        "true",
		"skip-verify": "skip-verify",
		"preferred":   "preferred",
	}
	for mode, want := range cases {
		cfg := baseMySQLConfig()
		cfg.TLS = mode
		dsn, err := buildMySQLDSN(cfg)
		if err != nil {
			t.Fatalf("tls mode %q: %v", mode, err)
		}
		parsed, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			t.Fatalf("parse dsn for tls mode %q: %v", mode, err)
		}
		if parsed.TLSConfig != want {
			t.Errorf("tls mode %q -> TLSConfig %q, want %q", mode, parsed.TLSConfig, want)
		}
	}
}

func TestBuildMySQLDSNInvalidTLSMode(t *testing.T) {
	cfg := baseMySQLConfig()
	cfg.TLS = "yes"
	if _, err := buildMySQLDSN(cfg); err == nil {
		t.Error("expected an error for unsupported tls value")
	}
}

func TestBuildMySQLDSNCustomTLSRequiresCAFile(t *testing.T) {
	cfg := baseMySQLConfig()
	cfg.TLS = "custom"
	if _, err := buildMySQLDSN(cfg); err == nil {
		t.Error("expected an error when custom tls has no CA file")
	}
}

func TestBuildMySQLDSNCustomTLSRegistersCA(t *testing.T) {
	caFile := writeTempCACertificate(t)
	cfg := baseMySQLConfig()
	cfg.TLS = "custom"
	cfg.TLSCAFile = caFile
	dsn, err := buildMySQLDSN(cfg)
	if err != nil {
		t.Fatalf("buildMySQLDSN: %v", err)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if parsed.TLSConfig != customMySQLTLSConfig {
		t.Fatalf("tls config = %q, want %q", parsed.TLSConfig, customMySQLTLSConfig)
	}
	// ParseDSN resolves a named TLS config through the driver's registry into
	// Config.TLS, which is the v1.7.0-visible proof that registration worked.
	if parsed.TLS == nil || parsed.TLS.RootCAs == nil {
		t.Fatal("custom tls config was not registered with a CA pool")
	}
	if len(parsed.TLS.RootCAs.Subjects()) == 0 {
		t.Error("registered CA pool contains no certificates")
	}
}

func TestBuildMySQLDSNCustomTLSMissingCAFile(t *testing.T) {
	cfg := baseMySQLConfig()
	cfg.TLS = "custom"
	cfg.TLSCAFile = filepath.Join(t.TempDir(), "missing.pem")
	if _, err := buildMySQLDSN(cfg); err == nil {
		t.Error("expected an error for a missing CA file")
	}
}

func TestBuildMySQLDSNCustomTLSRejectsInvalidPEM(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := baseMySQLConfig()
	cfg.TLS = "custom"
	cfg.TLSCAFile = caFile
	if _, err := buildMySQLDSN(cfg); err == nil {
		t.Error("expected an error for an invalid PEM CA file")
	}
}

func writeTempCACertificate(t *testing.T) string {
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
