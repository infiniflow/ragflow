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

package common

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const accessTokenExpireSeconds = 86400

const accessTokenContextVersion = "v2"

// AccessTokenContext is the identity and tenant context carried by a signed
// user token. TenantID is empty in Community Edition and for system admins.
type AccessTokenContext struct {
	AccessToken string
	TenantID    string
}

// ExtractAccessTokenContext extracts the access token and tenant context from
// an authorization header.
func ExtractAccessTokenContext(authorization, secretKey string) (*AccessTokenContext, error) {
	if authorization == "" {
		return nil, errors.New("empty authorization")
	}

	// Strip an optional Bearer scheme.
	token := strings.TrimSpace(authorization)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	parts := strings.Split(token, ".")
	if len(parts) != 5 || parts[0] != accessTokenContextVersion {
		return nil, errors.New("invalid access token format")
	}

	signature, err := urlSafeB64Decode(parts[4])
	if err != nil {
		return nil, fmt.Errorf("malformed token signature: %w", err)
	}
	value := strings.Join(parts[:4], ".")
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(value))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, errors.New("token signature does not match")
	}

	timestamp, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("malformed token timestamp: %w", err)
	}
	age := time.Now().Unix() - timestamp
	if age > accessTokenExpireSeconds {
		return nil, fmt.Errorf("token age %d > %d seconds", age, accessTokenExpireSeconds)
	}
	if age < 0 {
		return nil, fmt.Errorf("token age %d < 0 seconds", age)
	}

	accessTokenBytes, err := urlSafeB64Decode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("malformed access token: %w", err)
	}
	tenantIDBytes, err := urlSafeB64Decode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("malformed tenant context: %w", err)
	}
	if len(accessTokenBytes) == 0 {
		return nil, errors.New("empty access token")
	}

	return &AccessTokenContext{
		AccessToken: string(accessTokenBytes),
		TenantID:    string(tenantIDBytes),
	}, nil
}

// DumpAccessToken creates a signed authorization token from a user access
// token and its active tenant context.
func DumpAccessToken(accessToken, tenantID, secretKey string) (string, error) {
	if accessToken == "" {
		return "", errors.New("empty access token")
	}
	value := strings.Join([]string{
		accessTokenContextVersion,
		urlSafeB64Encode([]byte(accessToken)),
		urlSafeB64Encode([]byte(tenantID)),
		strconv.FormatInt(time.Now().Unix(), 10),
	}, ".")
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(value))
	return value + "." + urlSafeB64Encode(mac.Sum(nil)), nil
}

// urlSafeB64Decode decodes unpadded URL-safe base64.
func urlSafeB64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// urlSafeB64Encode encodes unpadded URL-safe base64.
func urlSafeB64Encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// GenerateSecretKey generates a 32-byte hex string (equivalent to Python's secrets.token_hex(32))
func GenerateSecretKey() (string, error) {
	byteArray := make([]byte, 32) // 32 bytes = 256 bits
	if _, err := rand.Read(byteArray); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}
	return hex.EncodeToString(byteArray), nil
}

func GenerateToken() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// GenerateUUID generates a UUID without dashes
func GenerateUUID() string {
	newID := strings.ReplaceAll(uuid.New().String(), "-", "")
	if len(newID) > 32 {
		newID = newID[:32]
	}
	return newID
}

// GenerateAPIToken generates secure random access key
func GenerateAPIToken() string {
	// Generate 32 random bytes
	bytesArray := make([]byte, 32)
	if _, err := rand.Read(bytesArray); err != nil {
		// Fallback to UUID if random generation fails
		return "ragflow-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	// Use URL-safe base64 encoding
	return "ragflow-" + base64.RawURLEncoding.EncodeToString(bytesArray)
}

// GenerateBetaAPIToken generates a beta access key
func GenerateBetaAPIToken() string {
	return GenerateUUID()
}
