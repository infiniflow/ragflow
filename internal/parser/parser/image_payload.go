package parser

import (
	"encoding/base64"
	"io"
	"strings"
)

// normalizeBase64ImagePayload validates an external image payload within its
// decoded byte limit and returns standard base64 with padding and no CR/LF.
// Validation streams into io.Discard instead of allocating decoded image bytes.
func normalizeBase64ImagePayload(payload string, maxDecodedBytes int) (string, bool) {
	if maxDecodedBytes < 0 {
		return "", false
	}
	maxEncodedBytes := base64.StdEncoding.EncodedLen(maxDecodedBytes)
	encodedBytes := 0
	for i := 0; i < len(payload); i++ {
		if payload[i] == '\r' || payload[i] == '\n' {
			continue
		}
		encodedBytes++
		if encodedBytes > maxEncodedBytes {
			return "", false
		}
	}
	if encodedBytes == 0 {
		return "", false
	}
	decodedSize, ok := base64DecodedSize(payload, encodedBytes)
	if !ok || decodedSize > int64(maxDecodedBytes) {
		return "", false
	}
	payload = strings.ReplaceAll(payload, "\r", "")
	payload = strings.ReplaceAll(payload, "\n", "")
	if remainder := encodedBytes % 4; remainder != 0 {
		payload += strings.Repeat("=", 4-remainder)
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	decodedBytes, err := io.CopyN(io.Discard, decoder, int64(maxDecodedBytes)+1)
	if err != io.EOF || decodedBytes == 0 || decodedBytes > int64(maxDecodedBytes) {
		return "", false
	}
	return payload, true
}

func base64DecodedSize(payload string, encodedBytes int) (int64, bool) {
	padding := 0
	sawPadding := false
	for i := 0; i < len(payload); i++ {
		if payload[i] == '\r' || payload[i] == '\n' {
			continue
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
	if encodedBytes%4 == 1 || (padding > 0 && encodedBytes%4 != 0) {
		return 0, false
	}
	decodedSize := int64(encodedBytes/4)*3 + int64(encodedBytes%4)*3/4 - int64(padding)
	return decodedSize, decodedSize >= 0
}
