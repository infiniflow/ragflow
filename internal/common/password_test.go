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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"testing"
)

const loginTestPassword = "Ragflow@hylink2026"

// encryptForLogin RSA-encrypts plain with conf/public.pem and base64s the
// ciphertext, which is the outer wrapper DecryptPassword expects.
func encryptForLogin(t *testing.T, plain string) string {
	t.Helper()
	pub, err := os.ReadFile("conf/public.pem")
	if err != nil {
		t.Fatalf("read public key: %v", err)
	}
	block, _ := pem.Decode(pub)
	if block == nil {
		t.Fatal("no PEM block in conf/public.pem")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, parsed.(*rsa.PublicKey), []byte(plain))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return base64.StdEncoding.EncodeToString(encrypted)
}

// inRepoRoot puts the working directory where LoadPrivateKey expects conf/.
func inRepoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
}

// Both the web client (rsaPsw) and the CLI (EncryptPassword) base64 the
// password before RSA, so this is the payload the API actually receives.
func TestDecryptPasswordFromWebClientAndCLI(t *testing.T) {
	inRepoRoot(t)

	payload := encryptForLogin(t, base64.StdEncoding.EncodeToString([]byte(loginTestPassword)))
	got, err := DecryptPassword(payload)
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if got != loginTestPassword {
		t.Errorf("DecryptPassword = %q, want %q", got, loginTestPassword)
	}
}

func TestDecryptPasswordUnwrapsInnerBase64(t *testing.T) {
	inRepoRoot(t)

	for _, password := range []string{"dGVzdA==", "test", "中文密码"} {
		payload := encryptForLogin(t, base64.StdEncoding.EncodeToString([]byte(password)))
		got, err := DecryptPassword(payload)
		if err != nil {
			t.Fatalf("DecryptPassword(%q): %v", password, err)
		}
		if got != password {
			t.Errorf("DecryptPassword(%q) = %q", password, got)
		}
	}
}

func TestDecryptPasswordFallsBackToPlaintext(t *testing.T) {
	// Not encrypted at all: the original value comes back untouched.
	got, err := DecryptPassword("not-a-base64-payload!!")
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if got != "not-a-base64-payload!!" {
		t.Errorf("DecryptPassword = %q", got)
	}
}
