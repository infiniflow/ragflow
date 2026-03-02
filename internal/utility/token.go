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

package utility

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/iromli/go-itsdangerous"
)

// ExtractAccessToken extract access token from authorization header
// This is equivalent to: str(jwt.loads(authorization)) in Python
// Uses github.com/iromli/go-itsdangerous for itsdangerous compatibility
func ExtractAccessToken(authorization, secretKey string) (string, error) {
	if authorization == "" {
		return "", errors.New("empty authorization")
	}

	// Create URLSafeTimedSerializer with correct configuration
	// Matching Python itsdangerous configuration:
	// - salt: "itsdangerous"
	// - key_derivation: "django-concat"
	// - digest_method: sha1
	algo := &itsdangerous.HMACAlgorithm{DigestMethod: sha1.New}
	signer := itsdangerous.NewTimestampSignature(
		secretKey,
		"itsdangerous",
		".",
		"django-concat",
		sha1.New,
		algo,
	)

	// Unsign the token (verifies signature and extracts payload)
	encodedValue, err := signer.Unsign(authorization, 0)
	if err != nil {
		return "", fmt.Errorf("failed to decode token: %w", err)
	}

	// Base64 decode the payload
	jsonValue, err := urlSafeB64Decode(encodedValue)
	if err != nil {
		return "", fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse JSON string (remove surrounding quotes)
	value := string(jsonValue)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		value = value[1 : len(value)-1]
	}

	return value, nil
}

// urlSafeB64Decode URL-safe base64 decode
func urlSafeB64Decode(s string) ([]byte, error) {
	// Add padding if needed
	padding := 4 - len(s)%4
	if padding != 4 {
		s += strings.Repeat("=", padding)
	}
	return base64.URLEncoding.DecodeString(s)
}

// generateSecretKey generates a 32-byte hex string (equivalent to Python's secrets.token_hex(32))
func GenerateSecretKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %v", err)
	}
	return hex.EncodeToString(bytes), nil
}
