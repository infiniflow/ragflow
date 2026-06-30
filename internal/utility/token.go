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

	"github.com/google/uuid"
	"github.com/iromli/go-itsdangerous"
)

// ExtractAccessToken extract access token from authorization header
// This is equivalent to: str(jwt.loads(authorization)) in Python
// Uses github.com/iromli/go-itsdangerous for itsdangerous compatibility
func ExtractAccessToken(authorization, secretKey string) (string, error) {
	if authorization == "" {
		return "", errors.New("empty authorization")
	}

	// Strip "Bearer " prefix if present
	token := strings.TrimPrefix(authorization, "Bearer ")

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
	encodedValue, err := signer.Unsign(token, 0)
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

// DumpAccessToken creates an authorization token from access token
// This is equivalent to: jwt.dumps(access_token) in Python
// Uses github.com/iromli/go-itsdangerous for itsdangerous compatibility
func DumpAccessToken(accessToken, secretKey string) (string, error) {
	if accessToken == "" {
		return "", errors.New("empty access token")
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

	// Encode the access token as JSON string (add surrounding quotes)
	jsonValue := fmt.Sprintf("\"%s\"", accessToken)
	encodedValue := urlSafeB64Encode([]byte(jsonValue))

	// Sign the token (creates signature)
	token, err := signer.Sign(encodedValue)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return token, nil
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

// urlSafeB64Encode URL-safe base64 encode (without padding)
func urlSafeB64Encode(data []byte) string {
	encoded := base64.URLEncoding.EncodeToString(data)
	// Remove padding
	return strings.TrimRight(encoded, "=")
}

// generateSecretKey generates a 32-byte hex string (equivalent to Python's secrets.token_hex(32))
func GenerateSecretKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %v", err)
	}
	return hex.EncodeToString(bytes), nil
}

func GenerateToken() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// GenerateAPIToken generates a secure random access key
// Equivalent to Python's generate_confirmation_token():
// return "ragflow-" + secrets.token_urlsafe(32)
func GenerateAPIToken() string {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to UUID if random generation fails
		return "ragflow-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	// Use URL-safe base64 encoding (same as Python's token_urlsafe)
	return "ragflow-" + base64.RawURLEncoding.EncodeToString(bytes)
}

// GenerateBetaAPIToken generates a beta access key
// Equivalent to Python's: generate_confirmation_token().replace("ragflow-", "")[:32]
func GenerateBetaAPIToken(accessKey string) string {
	// Remove "ragflow-" prefix
	withoutPrefix := strings.TrimPrefix(accessKey, "ragflow-")
	// Take first 32 characters
	if len(withoutPrefix) > 32 {
		return withoutPrefix[:32]
	}
	return withoutPrefix
}
