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
	"bytes"
	"compress/zlib"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

const accessTokenExpireSeconds = 86400

// ExtractAccessToken extract access token from authorization header
// This is equivalent to: str(jwt.loads(authorization)) in Python
func ExtractAccessToken(authorization, secretKey string) (string, error) {
	if authorization == "" {
		return "", errors.New("empty authorization")
	}

	// Strip "Bearer " prefix if present
	token := strings.TrimPrefix(authorization, "Bearer ")

	encodedValue, err := unsignAccessToken(token, secretKey, accessTokenExpireSeconds)
	if err != nil {
		return "", fmt.Errorf("failed to decode token: %w", err)
	}

	jsonValue, err := decodePayload(encodedValue)
	if err != nil {
		return "", fmt.Errorf("failed to decode payload: %w", err)
	}

	var value string
	if err := json.Unmarshal(jsonValue, &value); err != nil {
		return "", fmt.Errorf("failed to parse payload: %w", err)
	}

	return value, nil
}

// DumpAccessToken creates an authorization token from access token
// This is equivalent to: jwt.dumps(access_token) in Python
func DumpAccessToken(accessToken, secretKey string) (string, error) {
	if accessToken == "" {
		return "", errors.New("empty access token")
	}

	jsonValue, err := json.Marshal(accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to encode payload: %w", err)
	}

	return signAccessToken(encodePayload(jsonValue), secretKey), nil
}

// urlSafeB64Decode URL-safe base64 decode
func urlSafeB64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// urlSafeB64Encode URL-safe base64 encode (without padding)
func urlSafeB64Encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func encodePayload(jsonValue []byte) string {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(jsonValue)
	_ = zw.Close()

	compressed := buf.Bytes()
	if len(compressed) < len(jsonValue)-1 {
		return "." + urlSafeB64Encode(compressed)
	}
	return urlSafeB64Encode(jsonValue)
}

func decodePayload(payload string) ([]byte, error) {
	decompress := false
	if strings.HasPrefix(payload, ".") {
		payload = payload[1:]
		decompress = true
	}

	jsonValue, err := urlSafeB64Decode(payload)
	if err != nil {
		return nil, err
	}
	if !decompress {
		return jsonValue, nil
	}

	zr, err := zlib.NewReader(bytes.NewReader(jsonValue))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func signAccessToken(payload, secretKey string) string {
	value := payload + "." + urlSafeB64Encode(intToBytes(time.Now().Unix()))
	return value + "." + getAccessTokenSignature(value, secretKey)
}

func unsignAccessToken(token, secretKey string, maxAge int64) (string, error) {
	token = strings.TrimSpace(token)

	li := strings.LastIndex(token, ".")
	if li < 0 {
		return "", errors.New("signature missing")
	}
	value := token[:li]
	sig := token[li+1:]
	if !hmac.Equal([]byte(sig), []byte(getAccessTokenSignature(value, secretKey))) {
		return "", errors.New("signature does not match")
	}

	li = strings.LastIndex(value, ".")
	if li < 0 {
		return "", errors.New("timestamp missing")
	}
	payload := value[:li]
	tsValue := value[li+1:]
	tsBytes, err := urlSafeB64Decode(tsValue)
	if err != nil {
		return "", fmt.Errorf("malformed timestamp: %w", err)
	}
	timestamp := bytesToInt(tsBytes)
	if maxAge > 0 {
		age := time.Now().Unix() - timestamp
		if age > maxAge {
			return "", fmt.Errorf("signature age %d > %d seconds", age, maxAge)
		}
		if age < 0 {
			return "", fmt.Errorf("signature age %d < 0 seconds", age)
		}
	}
	return payload, nil
}

func getAccessTokenSignature(value, secretKey string) string {
	h := sha1.New()
	h.Write([]byte("itsdangerous" + "signer" + secretKey))
	key := h.Sum(nil)

	mac := hmac.New(sha1.New, key)
	mac.Write([]byte(value))
	return urlSafeB64Encode(mac.Sum(nil))
}

func intToBytes(num int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(num))
	return bytes.TrimLeft(buf[:], "\x00")
}

func bytesToInt(data []byte) int64 {
	if len(data) > 8 {
		data = data[len(data)-8:]
	}
	var buf [8]byte
	copy(buf[8-len(data):], data)
	return int64(binary.BigEndian.Uint64(buf[:]))
}

// GenerateSecretKey generates a 32-byte hex string (equivalent to Python's secrets.token_hex(32))
func GenerateSecretKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}
	return hex.EncodeToString(bytes), nil
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
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to UUID if random generation fails
		return "ragflow-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	// Use URL-safe base64 encoding
	return "ragflow-" + base64.RawURLEncoding.EncodeToString(bytes)
}

// GenerateBetaAPIToken generates a beta access key
func GenerateBetaAPIToken() string {
	return GenerateUUID()
}
