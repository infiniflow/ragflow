package service

import (
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"

	"ragflow/internal/utility"

	"github.com/zeebo/xxh3"
)

var (
	presentationUploadPattern = regexp.MustCompile(`(?i)\.(ppt|pptx|pages)$`)
	emailUploadPattern        = regexp.MustCompile(`(?i)\.(msg|eml)$`)
)

// selectUploadParser mirrors Python FileService.get_parser.
func selectUploadParser(docType utility.FileType, filename, defaultParser string) string {
	switch docType {
	case utility.FileTypeVISUAL:
		return "picture"
	case utility.FileTypeAURAL:
		return "audio"
	}
	base := filepath.Base(strings.TrimSpace(filename))
	switch {
	case presentationUploadPattern.MatchString(base):
		return "presentation"
	case emailUploadPattern.MatchString(base):
		return "email"
	default:
		return defaultParser
	}
}

// contentHashHex mirrors Python xxhash.xxh128(blob).hexdigest().
func contentHashHex(blob []byte) string {
	sum := xxh3.Hash128(blob).Bytes()
	return hex.EncodeToString(sum[:])
}
