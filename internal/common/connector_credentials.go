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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The Python server reads and writes the same format, so both must change together.
const (
	connectorCredentialsField    = "credentials"
	connectorCredentialsPrefix   = "enc:v1:"
	connectorCredentialsKeyBytes = 32
)

// Errors never carry the key, the ciphertext or the plaintext, because they reach API responses and task logs.
var (
	errConnectorKeyNotSet = errors.New("connector credentials are encrypted but " + EnvRAGFlowConnectorKey + " is not set")
	errConnectorDecrypt   = errors.New("cannot decrypt connector credentials: wrong " + EnvRAGFlowConnectorKey + " or corrupted value")
)

// ConnectorKey returns the AES-256 key in RAGFLOW_CONNECTOR_KEY, or nil when the variable is unset or empty.
func ConnectorKey() ([]byte, error) {
	encoded := GetEnv(EnvRAGFlowConnectorKey)
	if encoded == "" {
		return nil, nil
	}
	// Go's decoder skips \r and \n; reject them so a key is valid in Go only when it is valid in Python.
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || strings.ContainsAny(encoded, "\r\n") {
		return nil, fmt.Errorf("%s is not valid base64", EnvRAGFlowConnectorKey)
	}
	if len(key) != connectorCredentialsKeyBytes {
		return nil, fmt.Errorf("%s must decode to %d bytes, got %d", EnvRAGFlowConnectorKey, connectorCredentialsKeyBytes, len(key))
	}
	return key, nil
}

// EncryptConnectorCredentials returns a copy of config with config["credentials"] encrypted.
// It returns config itself when the key is unset, credentials are absent, or they are already encrypted.
func EncryptConnectorCredentials(config map[string]interface{}) (map[string]interface{}, error) {
	credentials, ok := config[connectorCredentialsField]
	if !ok {
		return config, nil
	}
	if value, isString := credentials.(string); isString && strings.HasPrefix(value, connectorCredentialsPrefix) {
		return config, nil
	}
	key, err := ConnectorKey()
	if err != nil {
		return nil, err
	}
	if key == nil {
		return config, nil
	}

	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return nil, fmt.Errorf("cannot encode connector credentials: %w", err)
	}
	gcm, err := connectorCredentialsCipher(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("cannot generate connector credentials nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return withConnectorCredentials(config, connectorCredentialsPrefix+base64.StdEncoding.EncodeToString(sealed)), nil
}

// DecryptConnectorCredentials returns a copy of config with encrypted credentials decoded.
// It returns config itself when the credentials are not encrypted.
func DecryptConnectorCredentials(config map[string]interface{}) (map[string]interface{}, error) {
	value, ok := config[connectorCredentialsField].(string)
	if !ok || !strings.HasPrefix(value, connectorCredentialsPrefix) {
		return config, nil
	}
	key, err := ConnectorKey()
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, errConnectorKeyNotSet
	}

	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, connectorCredentialsPrefix))
	if err != nil {
		return nil, errConnectorDecrypt
	}
	gcm, err := connectorCredentialsCipher(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errConnectorDecrypt
	}
	plaintext, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil {
		return nil, errConnectorDecrypt
	}
	var credentials interface{}
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, errConnectorDecrypt
	}
	return withConnectorCredentials(config, credentials), nil
}

// HasEncryptedConnectorCredentials reports whether config carries credentials in the stored "enc:" form.
// Requests must send plaintext: the server would store such a value as is and could not read it back.
func HasEncryptedConnectorCredentials(config map[string]interface{}) bool {
	value, ok := config[connectorCredentialsField].(string)
	return ok && strings.HasPrefix(value, "enc:")
}

func connectorCredentialsCipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// withConnectorCredentials copies config so callers never see their map change.
func withConnectorCredentials(config map[string]interface{}, credentials interface{}) map[string]interface{} {
	updated := make(map[string]interface{}, len(config))
	for key, value := range config {
		updated[key] = value
	}
	updated[connectorCredentialsField] = credentials
	return updated
}
