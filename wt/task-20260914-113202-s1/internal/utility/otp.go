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

// OTP / captcha helpers for the forgot-password flow.
// Constants and key shapes mirror api/utils/web_utils.py so the Python
// and Go backends share the same Redis namespace and contract.
package utility

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Forgot-password constants — match api/utils/web_utils.py.
const (
	OTPLength              = 4
	OTPTTL                 = 5 * time.Minute
	OTPAttemptLimit        = 5
	OTPAttemptLockDuration = 30 * time.Minute
	OTPResendCooldown      = 60 * time.Second
)

// otpUpperAlphabet is the OTP alphabet (uppercase letters, same as
// Python “string.ascii_uppercase“).
const otpUpperAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// captchaAlphabet is the captcha alphabet (uppercase letters + digits,
// same as Python “string.ascii_uppercase + string.digits“).
const captchaAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// normalizeEmail lowercases and trims an email address for keying. Mirrors
// the leading “email = (email or "").strip().lower()“ in Python's
// otp_keys helper.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// CaptchaIDRedisKey returns the Redis key that holds the active captcha
// for a server-issued captcha_id. The handler returns the id to the
// client and never the code itself, so an attacker cannot read the
// expected answer from the response. Diverges from Python's
// email-keyed “captcha_key“ on purpose — captchas are 60s-lived
// and never cross between Go and Python in practice, so there is no
// shared-state requirement.
func CaptchaIDRedisKey(captchaID string) string {
	return "captcha:" + captchaID
}

// OTPRedisKeys returns the four Redis keys used by the forgot-password
// flow, in the same order as Python's “otp_keys“ helper:
//
//	code, attempts, last_sent, lock
func OTPRedisKeys(email string) (codeKey, attemptsKey, lastSentKey, lockKey string) {
	email = normalizeEmail(email)
	return "otp:" + email,
		"otp_attempts:" + email,
		"otp_last_sent:" + email,
		"otp_lock:" + email
}

// OTPVerifiedRedisKey returns the Redis key that records a successful OTP
// verification, used as the gate for the password-reset step (matches
// Python “_verified_key“).
func OTPVerifiedRedisKey(email string) string {
	return "otp:verified:" + normalizeEmail(email)
}

// HashOTPCode computes the HMAC-SHA256 of an OTP using the given salt and
// returns its hex digest, matching Python's “hash_code“ helper.
func HashOTPCode(code string, salt []byte) string {
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateOTPSalt returns a cryptographically random 16-byte salt for
// hashing an OTP — same width as Python “os.urandom(16)“.
func GenerateOTPSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate otp salt: %w", err)
	}
	return salt, nil
}

// GenerateOTPCode generates an OTP of length “OTPLength“ drawn uniformly
// from “otpUpperAlphabet“ using crypto/rand (matches Python
// “secrets.choice“).
func GenerateOTPCode() (string, error) {
	return randomStringFromAlphabet(otpUpperAlphabet, OTPLength)
}

// GenerateCaptchaCode generates a captcha of length “OTPLength“ drawn
// uniformly from “captchaAlphabet“ using crypto/rand. The shared length
// is intentional — Python uses “OTP_LENGTH“ for both.
func GenerateCaptchaCode() (string, error) {
	return randomStringFromAlphabet(captchaAlphabet, OTPLength)
}

// EncodeOTPStorageValue serializes the (hash, salt) pair the way Python
// stores it in Redis: “"<hex_hash>:<hex_salt>"“. Returning the salt's
// hex form (not raw bytes) keeps the value safe to store as a Redis
// string and matches the Python encoding so either backend can verify a
// code minted by the other.
func EncodeOTPStorageValue(codeHash string, salt []byte) string {
	return codeHash + ":" + hex.EncodeToString(salt)
}

// DecodeOTPStorageValue reverses “EncodeOTPStorageValue“. Returns the
// stored hash, decoded salt bytes, and a non-nil error if the value is
// malformed.
func DecodeOTPStorageValue(stored string) (codeHash string, salt []byte, err error) {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("otp storage value missing salt separator")
	}
	salt, err = hex.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("otp storage salt is not valid hex: %w", err)
	}
	return parts[0], salt, nil
}

func randomStringFromAlphabet(alphabet string, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("random string length must be positive")
	}
	out := make([]byte, length)
	maxInt := big.NewInt(int64(len(alphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, maxInt)
		if err != nil {
			return "", fmt.Errorf("failed to read random byte: %w", err)
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}
