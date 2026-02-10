package cli

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// EncryptPassword encrypts a password using RSA public key
// This matches the Python implementation in api/utils/crypt.py
func EncryptPassword(password string) (string, error) {
	// Read public key from conf/public.pem
	publicKeyPath := filepath.Join(getProjectBaseDirectory(), "conf", "public.pem")
	publicKeyPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	// Parse public key
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return "", fmt.Errorf("failed to parse public key PEM")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Try parsing as PKCS1
		pub, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("failed to parse public key: %w", err)
		}
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("not an RSA public key")
	}

	// Step 1: Base64 encode the password
	passwordBase64 := base64.StdEncoding.EncodeToString([]byte(password))

	// Step 2: Encrypt using RSA PKCS1v15
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, []byte(passwordBase64))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt password: %w", err)
	}

	// Step 3: Base64 encode the encrypted data
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// getProjectBaseDirectory returns the project base directory
func getProjectBaseDirectory() string {
	// Try to find the project root by looking for go.mod or conf directory
	// Start from current working directory and go up
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}

	dir := cwd
	for {
		// Check if conf directory exists
		confDir := filepath.Join(dir, "conf")
		if info, err := os.Stat(confDir); err == nil && info.IsDir() {
			return dir
		}

		// Check for go.mod
		goMod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			return dir
		}

		// Go up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return cwd
}
