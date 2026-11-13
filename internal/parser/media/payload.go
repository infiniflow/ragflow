//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package media

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
)

// maxStreamBuffer caps the copy buffer used while validating a payload.
const maxStreamBuffer = 32 << 10

// NormalizeBase64ImagePayload validates an external image payload within its
// decoded byte limit and returns standard base64 with padding, no CR/LF, the
// decoded byte count, and the digest of the decoded bytes. Validation streams
// the payload instead of retaining a decoded image buffer, so the parser's
// normalization path can hash every image without holding the decoded bytes.
func NormalizeBase64ImagePayload(payload string, maxDecodedBytes int) (string, int64, [sha256.Size]byte, bool) {
	var sum [sha256.Size]byte
	payload, ok := cleanBase64ImagePayload(payload, maxDecodedBytes)
	if !ok {
		return "", 0, sum, false
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	hasher := sha256.New()
	limit := int64(maxDecodedBytes) + 1
	// io.CopyN would allocate a fresh 32 KiB buffer for every image, which
	// dominates both the parser and the fallback index walk on documents made
	// of small images. Size the copy buffer to the decoded payload instead.
	bufSize := int64(len(payload)/4*3) + 1
	if bufSize > maxStreamBuffer {
		bufSize = maxStreamBuffer
	}
	decodedBytes, err := io.CopyBuffer(io.MultiWriter(io.Discard, hasher), io.LimitReader(decoder, limit), make([]byte, bufSize))
	if err != nil || decodedBytes == 0 || decodedBytes > int64(maxDecodedBytes) {
		return "", 0, sum, false
	}
	copy(sum[:], hasher.Sum(nil))
	return payload, decodedBytes, sum, true
}

// decodeBase64ImagePayload validates and decodes one payload in a single
// pass, for callers that need the bytes. Digest calculation belongs to the
// media cache that stores those bytes.
func decodeBase64ImagePayload(payload string, maxDecodedBytes int) ([]byte, bool) {
	payload, ok := cleanBase64ImagePayload(payload, maxDecodedBytes)
	if !ok {
		return nil, false
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 || len(data) > maxDecodedBytes {
		return nil, false
	}
	return data, true
}

// cleanBase64ImagePayload applies the shared length/padding validation and
// returns the payload in the form both the streaming and one-shot decoders
// accept.
func cleanBase64ImagePayload(payload string, maxDecodedBytes int) (string, bool) {
	if maxDecodedBytes < 0 {
		return "", false
	}
	encodedBytes, _, ok := scanBase64ImagePayload(payload, base64.StdEncoding.EncodedLen(maxDecodedBytes))
	if !ok {
		return "", false
	}
	payload = strings.ReplaceAll(payload, "\r", "")
	payload = strings.ReplaceAll(payload, "\n", "")
	if remainder := encodedBytes % 4; remainder != 0 {
		payload += strings.Repeat("=", 4-remainder)
	}
	return payload, true
}

// scanBase64ImagePayload counts the base64 characters of an image payload
// (CR/LF excluded) and returns the padding count. It rejects bytes outside the
// standard base64 alphabet and a payload whose padding is malformed: '=' may
// only appear in the final quantum, at most twice, and the character count
// must form whole quanta.
//
// The padding rule is what rejects data that follows a padded quantum. Go's
// streaming decoder only reports that when the whole payload lands in one
// decode call, so it cannot be left to the decoder.
func scanBase64ImagePayload(payload string, maxEncodedBytes int) (encodedBytes, padding int, ok bool) {
	sawPadding := false
	for i := 0; i < len(payload); i++ {
		c := payload[i]
		if c == '\r' || c == '\n' {
			continue
		}
		encodedBytes++
		if encodedBytes > maxEncodedBytes {
			return 0, 0, false
		}
		if c == '=' {
			sawPadding = true
			padding++
			if padding > 2 {
				return 0, 0, false
			}
			continue
		}
		if sawPadding || !isBase64Alphabet(c) {
			return 0, 0, false
		}
	}
	if encodedBytes == 0 {
		return 0, 0, false
	}
	if encodedBytes%4 == 1 || (padding > 0 && encodedBytes%4 != 0) {
		return 0, 0, false
	}
	return encodedBytes, padding, true
}

// isBase64Alphabet reports whether c is a standard base64 alphabet byte.
func isBase64Alphabet(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/':
		return true
	}
	return false
}
