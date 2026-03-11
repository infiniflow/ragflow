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
	"hash"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// CheckWerkzeugPassword verifies a password against a werkzeug password hash
// Format: pbkdf2:sha256:iterations$salt$hash
func CheckWerkzeugPassword(password, hashStr string) bool {
	parts := strings.Split(hashStr, "$")
	if len(parts) != 3 {
		return false
	}

	// Parse method (e.g., "pbkdf2:sha256:150000")
	methodParts := strings.Split(parts[0], ":")
	if len(methodParts) != 3 {
		return false
	}

	if methodParts[0] != "pbkdf2" {
		return false
	}

	var hashFunc func() hash.Hash
	switch methodParts[1] {
	case "sha256":
		hashFunc = sha256.New
	case "sha512":
		// sha512 not supported in this implementation
		return false
	default:
		return false
	}

	iterations, err := strconv.Atoi(methodParts[2])
	if err != nil {
		return false
	}

	salt := parts[1]
	expectedHash := parts[2]

	// Decode salt from base64
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		// Try hex encoding
		saltBytes, err = hex.DecodeString(salt)
		if err != nil {
			return false
		}
	}

	// Generate hash using PBKDF2
	key := pbkdf2.Key([]byte(password), saltBytes, iterations, 32, hashFunc)
	computedHash := base64.StdEncoding.EncodeToString(key)

	return computedHash == expectedHash
}

// IsWerkzeugHash checks if a hash is in werkzeug format
func IsWerkzeugHash(hashStr string) bool {
	return strings.HasPrefix(hashStr, "pbkdf2:")
}

// GenerateWerkzeugPasswordHash generates a werkzeug-compatible password hash
func GenerateWerkzeugPasswordHash(password string, iterations int) (string, error) {
	if iterations == 0 {
		iterations = 150000
	}

	// Generate random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// Generate hash using PBKDF2-SHA256
	key := pbkdf2.Key([]byte(password), salt, iterations, 32, sha256.New)

	// Format: pbkdf2:sha256:iterations$base64(salt)$base64(hash)
	saltB64 := base64.StdEncoding.EncodeToString(salt)
	hashB64 := base64.StdEncoding.EncodeToString(key)

	return fmt.Sprintf("pbkdf2:sha256:%d$%s$%s", iterations, saltB64, hashB64), nil
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
