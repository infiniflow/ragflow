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

package admin

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

// CheckWerkzeugPassword verifies a password against a werkzeug password hash
// Supports both pbkdf2 and scrypt formats
func CheckWerkzeugPassword(password, hashStr string) bool {
	if strings.HasPrefix(hashStr, "scrypt:") {
		return checkScryptPassword(password, hashStr)
	}
	if strings.HasPrefix(hashStr, "pbkdf2:") {
		return checkPBKDF2Password(password, hashStr)
	}
	return false
}

// checkScryptPassword verifies password using scrypt format
// Format: scrypt:n:r:p$base64(salt)$hex(hash)
// IMPORTANT: werkzeug uses the base64-encoded salt string as UTF-8 bytes, NOT the decoded bytes
func checkScryptPassword(password, hashStr string) bool {
	parts := strings.Split(hashStr, "$")
	if len(parts) != 3 {
		return false
	}

	params := strings.Split(parts[0], ":")
	if len(params) != 4 || params[0] != "scrypt" {
		return false
	}

	n, err := strconv.ParseUint(params[1], 10, 0)
	if err != nil {
		return false
	}
	r, err := strconv.ParseUint(params[2], 10, 0)
	if err != nil {
		return false
	}
	p, err := strconv.ParseUint(params[3], 10, 0)
	if err != nil {
		return false
	}

	saltB64 := parts[1]
	hashHex := parts[2]

	// IMPORTANT: werkzeug uses the base64 string as UTF-8 bytes, NOT decoded bytes
	// This is the key difference from standard implementations
	salt := []byte(saltB64)

	// Decode hash from hex
	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}

	if n > math.MaxInt || r > math.MaxInt || p > math.MaxInt {
		return false
	}
	computed, err := scrypt.Key([]byte(password), salt, int(n), int(r), int(p), len(expectedHash))
	if err != nil {
		return false
	}

	return constantTimeCompare(expectedHash, computed)
}

// checkPBKDF2Password verifies password using PBKDF2 format
// Format: pbkdf2:sha256:iterations$base64(salt)$base64(hash)
func checkPBKDF2Password(password, hashStr string) bool {
	parts := strings.Split(hashStr, "$")
	if len(parts) != 3 {
		return false
	}

	methodParts := strings.Split(parts[0], ":")
	if len(methodParts) != 3 || methodParts[0] != "pbkdf2" {
		return false
	}

	iterations, err := strconv.Atoi(methodParts[2])
	if err != nil {
		return false
	}

	salt := parts[1]
	expectedHash := parts[2]

	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		saltBytes, err = hex.DecodeString(salt)
		if err != nil {
			return false
		}
	}

	key := pbkdf2.Key([]byte(password), saltBytes, iterations, 32, sha256.New)
	computedHash := base64.StdEncoding.EncodeToString(key)

	return computedHash == expectedHash
}

// constantTimeCompare performs constant time comparison
func constantTimeCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

// IsWerkzeugHash checks if a hash is in werkzeug format
func IsWerkzeugHash(hashStr string) bool {
	return strings.HasPrefix(hashStr, "scrypt:") || strings.HasPrefix(hashStr, "pbkdf2:")
}

// GenerateWerkzeugPasswordHash generates a werkzeug-compatible password hash using scrypt
// This matches Python werkzeug's default behavior
func GenerateWerkzeugPasswordHash(password string, iterations int) (string, error) {
	// Generate random bytes (12 bytes will produce 16-char base64 string)
	randomBytes := make([]byte, 12)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// Encode to base64 string (this will be 16 characters)
	saltB64 := base64.StdEncoding.EncodeToString(randomBytes)

	// Use scrypt with werkzeug default parameters: N=32768, r=8, p=1, keyLen=64
	// IMPORTANT: werkzeug uses the base64 string as UTF-8 bytes, NOT the decoded bytes
	hash, err := scrypt.Key([]byte(password), []byte(saltB64), 32768, 8, 1, 64)
	if err != nil {
		return "", err
	}

	// Format: scrypt:n:r:p$base64(salt)$hex(hash)
	return fmt.Sprintf("scrypt:32768:8:1$%s$%x", saltB64, hash), nil
}

// DecryptPassword decrypts the password using RSA private key
// The password is expected to be base64 encoded RSA encrypted data
// If decryption fails, the original password is returned (assumed to be plain text)
func DecryptPassword(encryptedPassword string) (string, error) {
	// Try to decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedPassword)
	if err != nil {
		// If base64 decoding fails, assume it's already a plain password
		return encryptedPassword, nil
	}

	// Load private key
	privateKey, err := loadPrivateKey()
	if err != nil {
		return "", err
	}

	// Decrypt using PKCS#1 v1.5
	plaintext, err := rsa.DecryptPKCS1v15(nil, privateKey, ciphertext)
	if err != nil {
		// If decryption fails, assume it's already a plain password
		return encryptedPassword, nil
	}

	return string(plaintext), nil
}

// loadPrivateKey loads and decrypts the RSA private key from conf/private.pem
func loadPrivateKey() (*rsa.PrivateKey, error) {
	// Read private key file
	keyData, err := os.ReadFile("conf/private.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	// Decrypt the PEM block if it's encrypted
	var privateKey interface{}
	if block.Headers["Proc-Type"] == "4,ENCRYPTED" {
		// Decrypt using password "Welcome"
		decryptedData, err := x509.DecryptPEMBlock(block, []byte("Welcome"))
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt private key: %w", err)
		}

		// Parse the decrypted key
		privateKey, err = x509.ParsePKCS1PrivateKey(decryptedData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	} else {
		// Not encrypted, parse directly
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	}

	rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an RSA private key")
	}

	return rsaPrivateKey, nil
}
