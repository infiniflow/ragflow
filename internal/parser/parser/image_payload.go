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

package parser

import (
	"encoding/base64"
	"io"
	"strings"
)

// normalizeBase64ImagePayload validates an external image payload within its
// decoded byte limit and returns standard base64 with padding, no CR/LF, and
// the decoded byte count. Validation streams into io.Discard instead of
// allocating decoded image buffers.
func normalizeBase64ImagePayload(payload string, maxDecodedBytes int) (string, int64, bool) {
	if maxDecodedBytes < 0 {
		return "", 0, false
	}
	encodedBytes, ok := scanBase64ImagePayload(payload, base64.StdEncoding.EncodedLen(maxDecodedBytes))
	if !ok {
		return "", 0, false
	}
	payload = strings.ReplaceAll(payload, "\r", "")
	payload = strings.ReplaceAll(payload, "\n", "")
	if remainder := encodedBytes % 4; remainder != 0 {
		payload += strings.Repeat("=", 4-remainder)
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	decodedBytes, err := io.CopyN(io.Discard, decoder, int64(maxDecodedBytes)+1)
	if err != io.EOF || decodedBytes == 0 || decodedBytes > int64(maxDecodedBytes) {
		return "", 0, false
	}
	return payload, decodedBytes, true
}

// scanBase64ImagePayload counts the base64 characters of an image payload
// (CR/LF excluded) and rejects a payload whose padding is malformed: '=' may
// only appear in the final quantum, at most twice, and the character count
// must form whole quanta.
//
// The padding rule is what rejects data that follows a padded quantum. Go's
// streaming decoder only reports that when the whole payload lands in one
// decode call, so it cannot be left to the decoder.
func scanBase64ImagePayload(payload string, maxEncodedBytes int) (int, bool) {
	encodedBytes := 0
	padding := 0
	sawPadding := false
	for i := 0; i < len(payload); i++ {
		if payload[i] == '\r' || payload[i] == '\n' {
			continue
		}
		encodedBytes++
		if encodedBytes > maxEncodedBytes {
			return 0, false
		}
		if payload[i] == '=' {
			sawPadding = true
			padding++
			if padding > 2 {
				return 0, false
			}
			continue
		}
		if sawPadding {
			return 0, false
		}
	}
	if encodedBytes == 0 {
		return 0, false
	}
	if encodedBytes%4 == 1 || (padding > 0 && encodedBytes%4 != 0) {
		return 0, false
	}
	return encodedBytes, true
}
