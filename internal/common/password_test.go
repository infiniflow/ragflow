// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// setupTestPrivateKey generates an RSA key pair, writes the private key to
// conf/private.pem under a temp dir, and chdirs the test there so
// LoadPrivateKey picks it up. It returns the public key for encryption.
func setupTestPrivateKey(t *testing.T) *rsa.PublicKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conf", "private.pem"), pemBytes, 0o600); err != nil {
		t.Fatalf("write private.pem: %v", err)
	}
	t.Chdir(dir)

	return &key.PublicKey
}

// encryptPassword mimics the SPA rsaPsw: base64-encode the password, RSA
// encrypt it, then base64-encode the ciphertext for transport.
func encryptPassword(t *testing.T, pub *rsa.PublicKey, password string) string {
	t.Helper()

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(base64.StdEncoding.EncodeToString([]byte(password))))
	if err != nil {
		t.Fatalf("RSA encrypt: %v", err)
	}
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// encryptRaw mimics a client sending RSA(plaintext): RSA encrypt the raw
// password, then base64-encode the ciphertext for transport.
func encryptRaw(t *testing.T, pub *rsa.PublicKey, password string) string {
	t.Helper()

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(password))
	if err != nil {
		t.Fatalf("RSA encrypt: %v", err)
	}
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func TestDecryptPasswordSPAPayload(t *testing.T) {
	pub := setupTestPrivateKey(t)

	// Regression test for infiniflow/ragflow#22570: the SPA sends
	// RSA(Base64(password)); the Go port dropped the inner base64 decode
	// that the Python-era decrypt2 performed, so login compared the
	// werkzeug hash against Base64(password) and always failed with 109
	// for accounts created from the raw password.
	const password = "Ragflow@hylink2026"
	got, err := DecryptPassword(encryptPassword(t, pub, password))
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if got != password {
		t.Fatalf("DecryptPassword(RSA(Base64(%q))) = %q, want %q", password, got, password)
	}
}

func TestDecryptPasswordNonASCII(t *testing.T) {
	pub := setupTestPrivateKey(t)

	const password = "pässwörd✓123"
	got, err := DecryptPassword(encryptPassword(t, pub, password))
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if got != password {
		t.Fatalf("DecryptPassword(RSA(Base64(%q))) = %q, want %q", password, got, password)
	}
}

func TestDecryptPasswordRawRSAPayload(t *testing.T) {
	pub := setupTestPrivateKey(t)

	// Clients sending RSA(plaintext) (e.g. curl repro in #22570) must keep
	// working: the plaintext is returned as-is when it is not valid
	// base64, or when it is not the canonical base64 of the password.
	const password = "Ragflow@hylink2026"
	got, err := DecryptPassword(encryptRaw(t, pub, password))
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if got != password {
		t.Fatalf("DecryptPassword(RSA(%q)) = %q, want %q", password, got, password)
	}
}

func TestDecryptPasswordPlaintextPassthrough(t *testing.T) {
	setupTestPrivateKey(t)

	// Not base64 and not RSA ciphertext: returned unchanged.
	const password = "not-encrypted-at-all"
	got, err := DecryptPassword(password)
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if got != password {
		t.Fatalf("DecryptPassword(%q) = %q, want %q", password, got, password)
	}
}

func TestDecryptPasswordCandidatesSPAPayload(t *testing.T) {
	pub := setupTestPrivateKey(t)

	// SPA payload: decoded password first (accounts created from the raw
	// password), raw base64 form second (accounts registered through the
	// SPA before the inner decode was restored).
	const password = "Ragflow@hylink2026"
	got, err := DecryptPasswordCandidates(encryptPassword(t, pub, password))
	if err != nil {
		t.Fatalf("DecryptPasswordCandidates: %v", err)
	}
	want := []string{password, base64.StdEncoding.EncodeToString([]byte(password))}
	if len(got) != len(want) {
		t.Fatalf("DecryptPasswordCandidates(...) = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DecryptPasswordCandidates(...) = %q, want %q", got, want)
		}
	}
}

func TestDecryptPasswordCandidatesRawRSAPayload(t *testing.T) {
	pub := setupTestPrivateKey(t)

	// RSA(plaintext) has a single candidate: the plaintext itself.
	const password = "Ragflow@hylink2026"
	got, err := DecryptPasswordCandidates(encryptRaw(t, pub, password))
	if err != nil {
		t.Fatalf("DecryptPasswordCandidates: %v", err)
	}
	if len(got) != 1 || got[0] != password {
		t.Fatalf("DecryptPasswordCandidates(RSA(%q)) = %q, want [%q]", password, got, password)
	}
}

func TestDecryptPasswordCandidatesPlaintextPassthrough(t *testing.T) {
	setupTestPrivateKey(t)

	const password = "not-encrypted-at-all"
	got, err := DecryptPasswordCandidates(password)
	if err != nil {
		t.Fatalf("DecryptPasswordCandidates: %v", err)
	}
	if len(got) != 1 || got[0] != password {
		t.Fatalf("DecryptPasswordCandidates(%q) = %q, want [%q]", password, got, password)
	}
}

func TestDecryptPasswordCandidatesMatchBothHashPopulations(t *testing.T) {
	pub := setupTestPrivateKey(t)

	// End-to-end proof for the two account populations in #22570: the
	// first candidate must verify against a hash of the raw password
	// (bootstrap superuser, API/CLI-created accounts) and the second
	// against a hash of the base64 form (accounts registered through the
	// SPA before the inner decode was restored).
	const password = "Ragflow@hylink2026"
	candidates, err := DecryptPasswordCandidates(encryptPassword(t, pub, password))
	if err != nil {
		t.Fatalf("DecryptPasswordCandidates: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("DecryptPasswordCandidates(...) = %q, want 2 candidates", candidates)
	}

	rawHash, err := GenerateWerkzeugPasswordHash(password)
	if err != nil {
		t.Fatalf("GenerateWerkzeugPasswordHash: %v", err)
	}
	if !CheckWerkzeugPassword(candidates[0], rawHash) {
		t.Fatalf("candidate[0] %q does not verify against hash of raw password", candidates[0])
	}

	b64Hash, err := GenerateWerkzeugPasswordHash(base64.StdEncoding.EncodeToString([]byte(password)))
	if err != nil {
		t.Fatalf("GenerateWerkzeugPasswordHash: %v", err)
	}
	if !CheckWerkzeugPassword(candidates[1], b64Hash) {
		t.Fatalf("candidate[1] %q does not verify against hash of base64 password", candidates[1])
	}
}
