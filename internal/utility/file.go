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

package utility

import (
	"path/filepath"
	"regexp"
	"strings"
)

const (
	FileTypePDF    = "pdf"
	FileTypeDOC    = "doc"
	FileTypeVISUAL = "visual"
	FileTypeAURAL  = "aural"
	FileTypeFOLDER = "folder"
	FileTypeOTHER  = "other"
)

var (
	filenameLenLimit = 255
)

func init() {
}

func normalizeFilename(filename string) (string, bool) {
	if filename == "" {
		return "", false
	}
	base := filepath.Base(filename)
	base = strings.TrimSpace(base)
	if base == "" || len(base) > filenameLenLimit {
		return "", false
	}
	return strings.ToLower(base), true
}

func FilenameType(filename string) string {
	normalized, ok := normalizeFilename(filename)
	if !ok {
		return FileTypeOTHER
	}

	if matched, _ := regexp.MatchString(`.*\.pdf$`, normalized); matched {
		return FileTypePDF
	}

	docExtensions := []string{
		"msg", "eml", "doc", "docx", "ppt", "pptx", "yml", "xml", "htm", "json", "jsonl", "ldjson",
		"csv", "txt", "ini", "xls", "xlsx", "wps", "rtf", "hlp", "pages", "numbers", "key",
		"md", "mdx", "py", "js", "java", "c", "cpp", "h", "php", "go", "ts", "sh", "cs", "kt",
		"html", "sql", "epub",
	}
	for _, ext := range docExtensions {
		if strings.HasSuffix(normalized, "."+ext) {
			return FileTypeDOC
		}
	}

	audioExtensions := []string{
		"wav", "flac", "ape", "alac", "wv", "mp3", "aac", "ogg", "vorbis", "opus",
	}
	for _, ext := range audioExtensions {
		if strings.HasSuffix(normalized, "."+ext) {
			return FileTypeAURAL
		}
	}

	visualExtensions := []string{
		"jpg", "jpeg", "png", "tif", "gif", "pcx", "tga", "exif", "fpx", "svg", "psd", "cdr",
		"pcd", "dxf", "ufo", "eps", "ai", "raw", "WMF", "webp", "avif", "apng", "icon", "ico",
		"mpg", "mpeg", "avi", "rm", "rmvb", "mov", "wmv", "asf", "dat", "asx", "wvx", "mpe",
		"mpa", "mp4", "mkv",
	}
	for _, ext := range visualExtensions {
		if strings.HasSuffix(normalized, "."+ext) {
			return FileTypeVISUAL
		}
	}

	return FileTypeOTHER
}

func SanitizeFilename(filename string) string {
	if filename == "" {
		return ""
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return ""
	}

	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = strings.Trim(filename, "/")

	parts := strings.Split(filename, "/")
	var sanitizedParts []string
	for _, part := range parts {
		if part != "" && part != "." && part != ".." {
			sanitizedParts = append(sanitizedParts, part)
		}
	}

	unsafeRegex := regexp.MustCompile(`[^A-Za-z0-9_\-/]`)
	for i, part := range sanitizedParts {
		sanitizedParts[i] = unsafeRegex.ReplaceAllString(part, "")
	}

	result := strings.Join(sanitizedParts, "/")
	return result
}

func GetFileExtension(filename string) string {
	ext := filepath.Ext(filename)
	if len(ext) > 0 && ext[0] == '.' {
		return strings.ToLower(ext[1:])
	}
	return strings.ToLower(ext)
}

// CONTENT_TYPE_MAP maps file extensions to MIME content types
var CONTENT_TYPE_MAP = map[string]string{
	// Office
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"doc":  "application/msword",
	"pdf":  "application/pdf",
	"csv":  "text/csv",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	// Text/code
	"txt":  "text/plain",
	"py":   "text/plain",
	"js":   "text/plain",
	"java": "text/plain",
	"c":    "text/plain",
	"cpp":  "text/plain",
	"h":    "text/plain",
	"php":  "text/plain",
	"go":   "text/plain",
	"ts":   "text/plain",
	"sh":   "text/plain",
	"cs":   "text/plain",
	"kt":   "text/plain",
	"sql":  "text/plain",
	// Web
	"md":       "text/markdown",
	"markdown": "text/markdown",
	"mdx":      "text/markdown",
	"htm":      "text/html",
	"html":     "text/html",
	"json":     "application/json",
	// Image formats
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"gif":  "image/gif",
	"bmp":  "image/bmp",
	"tiff": "image/tiff",
	"tif":  "image/tiff",
	"webp": "image/webp",
	"svg":  "image/svg+xml",
	"ico":  "image/x-icon",
	"avif": "image/avif",
	"heic": "image/heic",
	// PPTX
	"ppt":  "application/vnd.ms-powerpoint",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
}

// FORCE_ATTACHMENT_EXTENSIONS are extensions that should always be downloaded as attachments
var FORCE_ATTACHMENT_EXTENSIONS = map[string]bool{
	"htm":   true,
	"html":  true,
	"shtml": true,
	"xht":   true,
	"xhtml": true,
	"xml":   true,
	"mhtml": true,
	"svg":   true,
}

// FORCE_ATTACHMENT_CONTENT_TYPES are content types that should always be downloaded as attachments
var FORCE_ATTACHMENT_CONTENT_TYPES = map[string]bool{
	"text/html":             true,
	"image/svg+xml":         true,
	"application/xhtml+xml": true,
	"text/xml":              true,
	"application/xml":        true,
	"multipart/related":     true,
}

// ShouldForceAttachment determines if the file should be forced as attachment
func ShouldForceAttachment(ext string, contentType string) bool {
	normalizedExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	if normalizedExt != "" && FORCE_ATTACHMENT_EXTENSIONS[normalizedExt] {
		return true
	}
	normalizedType := strings.ToLower(contentType)
	return FORCE_ATTACHMENT_CONTENT_TYPES[normalizedType]
}

// GetContentType determines the content type based on extension and file type
// fallbackPrefix is "image" for visual files, "application" for others
func GetContentType(ext string, fileType string) string {
	if ext == "" {
		return ""
	}
	normalizedExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	if contentType, ok := CONTENT_TYPE_MAP[normalizedExt]; ok {
		return contentType
	}
	fallbackPrefix := "application"
	if fileType == FileTypeVISUAL {
		fallbackPrefix = "image"
	}
	return fallbackPrefix + "/" + normalizedExt
}
