package common

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func TestCheckWerkzeugPasswordAcceptsLegacyPBKDF2Algorithms(t *testing.T) {
	const password = "password-format-fixture"
	const iterations = 1000
	salt := []byte("legacy-fixture-salt")
	for _, algorithm := range []struct {
		name string
		sum  func() hash.Hash
	}{
		{"sha256", sha256.New},
		{"sha512", sha512.New},
	} {
		t.Run(algorithm.name, func(t *testing.T) {
			derived := pbkdf2.Key([]byte(password), salt, iterations, 32, algorithm.sum)
			stored := fmt.Sprintf("pbkdf2:%s:%d$%s$%s", algorithm.name, iterations,
				base64.StdEncoding.EncodeToString(salt), base64.StdEncoding.EncodeToString(derived))
			if !CheckWerkzeugPassword(password, stored) {
				t.Fatal("expected the legacy password format to verify")
			}
			if CheckWerkzeugPassword("wrong-password-fixture", stored) {
				t.Fatal("incorrect password was accepted")
			}
		})
	}
}
