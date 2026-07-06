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
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

type FileType string

const (
	FileTypePDF      FileType = "pdf"
	FileTypeDOC      FileType = "doc"
	FileTypeDOCX     FileType = "docx"
	FileTypePPT      FileType = "ppt"
	FileTypePPTX     FileType = "pptx"
	FileTypeXLS      FileType = "xls"
	FileTypeXLSX     FileType = "xlsx"
	FileTypeHTML     FileType = "html"
	FileTypeMarkdown FileType = "md"
	FileTypeTXT      FileType = "txt"
	FileTypeVISUAL   FileType = "visual"
	FileTypeAURAL    FileType = "aural"
	FileTypeFOLDER   FileType = "folder"
	FileTypeOTHER    FileType = "other"
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

func GetFileType(filename string) FileType {

	ext := filepath.Ext(filename)
	var suffix string
	if len(ext) > 0 && ext[0] == '.' {
		suffix = strings.ToLower(ext[1:])
	} else {
		suffix = strings.ToLower(ext)
	}

	switch suffix {
	case "pdf":
		return FileTypePDF
	case "xls":
		return FileTypeXLS
	case "xlsx":
		return FileTypeXLSX
	case "doc":
		return FileTypeDOC
	case "docx":
		return FileTypeDOCX
	case "ppt":
		return FileTypePPT
	case "pptx":
		return FileTypePPTX
	case "html", "htm":
		return FileTypeHTML
	case "md":
		return FileTypeMarkdown
	case "txt":
		return FileTypeTXT
	default:
		return FileTypeOTHER
	}
}

func FilenameType(filename string) FileType {
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
	// Video formats
	"mp4":  "video/mp4",
	"mov":  "video/quicktime",
	"avi":  "video/x-msvideo",
	"mpg":  "video/mpeg",
	"mpeg": "video/mpeg",
	"mkv":  "video/x-matroska",
	"wmv":  "video/x-ms-wmv",
	"webm": "video/webm",
	"rm":   "application/vnd.rn-realmedia",
	"rmvb": "application/vnd.rn-realmedia",
	"dat":  "video/mpeg",
	"asx":  "video/x-ms-asf",
	"wvx":  "video/x-ms-wvx",
	"mpe":  "video/mpeg",
	"mpa":  "video/mpeg",
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
	"application/xml":       true,
	"multipart/related":     true,
}

// stripContentTypeParams strips "; charset=..." and similar parameters
// from a content type string.  Mirrors Python's .split(";")[0].strip().
func stripContentTypeParams(ct string) string {
	if before, _, found := strings.Cut(ct, ";"); found {
		return strings.TrimSpace(before)
	}
	return strings.TrimSpace(ct)
}

// ShouldForceAttachment determines if the file should be forced as attachment
func ShouldForceAttachment(ext string, contentType string) bool {
	normalizedExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	if normalizedExt != "" && FORCE_ATTACHMENT_EXTENSIONS[normalizedExt] {
		return true
	}
	normalizedType := strings.ToLower(stripContentTypeParams(contentType))
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
	if fileType == string(FileTypeVISUAL) {
		fallbackPrefix = "image"
	}
	return fallbackPrefix + "/" + normalizedExt
}

// SanitizeContentDispositionFilename sanitizes a filename for use in
// Content-Disposition headers. Strips non-ASCII, path separators,
// control characters, and quotes/percent signs. Falls back to "file"
// when the result is empty. Mirrors Python file_response.py:
// sanitize_content_disposition_filename().
func SanitizeContentDispositionFilename(filename string) string {
	if filename == "" {
		return "file"
	}
	// Strip non-ASCII.
	var asciiOnly strings.Builder
	for _, r := range filename {
		if r < 0x80 {
			asciiOnly.WriteRune(r)
		}
	}
	sanitized := asciiOnly.String()

	// Replace path separators, special chars.
	sanitized = strings.ReplaceAll(sanitized, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, "\\", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	sanitized = strings.ReplaceAll(sanitized, "\"", "")
	sanitized = strings.ReplaceAll(sanitized, "'", "")
	sanitized = strings.ReplaceAll(sanitized, "%", "")

	// Strip remaining control characters.
	ctrlRe := regexp.MustCompile(`[\x00-\x1f\x7f]`)
	sanitized = ctrlRe.ReplaceAllString(sanitized, "")

	sanitized = strings.TrimSpace(sanitized)
	if sanitized == "" {
		return "file"
	}
	return sanitized
}

// ResolveAttachmentContentType resolves a content type and extension from
// query-parameter values. When mimeType is non-empty it is preferred and
// reverse-looked up in CONTENT_TYPE_MAP to also resolve the extension;
// otherwise the extension is looked up in CONTENT_TYPE_MAP, falling back
// to "application/<ext>". Returns (contentType, ext). Mirrors Python
// file_response.py: resolve_attachment_content_type().
func ResolveAttachmentContentType(ext string, mimeType string) (string, string) {
	ext = strings.ToLower(strings.TrimSpace(ext))
	ext = strings.TrimPrefix(ext, ".")
	mimeType = strings.TrimSpace(mimeType)

	contentType := ""
	if mimeType != "" {
		normalizedType := strings.ToLower(stripContentTypeParams(mimeType))
		contentType = normalizedType
		// Reverse-lookup extension from CONTENT_TYPE_MAP only when
		// no explicit ext was provided. Never overwrite a caller-
		// supplied extension (e.g. ext=svg&mime_type=image/png must
		// stay ext=svg for the force-attachment check).
		if ext == "" {
			for knownExt, knownType := range CONTENT_TYPE_MAP {
				if knownType == normalizedType {
					ext = knownExt
					break
				}
			}
		}
	} else if ext != "" {
		if ct, ok := CONTENT_TYPE_MAP[ext]; ok {
			contentType = ct
		} else {
			contentType = "application/" + ext
		}
	}
	return contentType, ext
}

// SetPreviewFileResponseHeaders sets response headers for inline file
// preview. For force-attachment types (HTML, SVG, XML) it falls back to
// attachment disposition with nosniff. Mirrors Python file_response.py:
// apply_preview_file_response_headers().
func SetPreviewFileResponseHeaders(h http.Header, contentType, ext, filename string) {
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	if ShouldForceAttachment(ext, contentType) {
		h.Set("Content-Disposition", "attachment")
		h.Set("X-Content-Type-Options", "nosniff")
	} else {
		safe := SanitizeContentDispositionFilename(filename)
		h.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safe))
	}
}

// SetDownloadFileResponseHeaders sets response headers for file download
// (always attachment disposition). Mirrors Python file_response.py:
// apply_download_file_response_headers().
func SetDownloadFileResponseHeaders(h http.Header, contentType, ext, filename string) {
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	if ShouldForceAttachment(ext, contentType) {
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Disposition", "attachment")
		return
	}
	safe := SanitizeContentDispositionFilename(filename)
	h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safe))
}

// AgentAttachmentPreviewPath builds the preview URL path for an agent
// attachment. Query parameters ext and mime_type are URL-encoded when
// provided. Mirrors Python file_response.py:
// agent_attachment_preview_path().
func AgentAttachmentPreviewPath(attachmentID, ext, mimeType string) string {
	path := "/api/v1/agents/attachments/" + attachmentID + "/preview"
	params := url.Values{}
	if ext != "" {
		params.Set("ext", ext)
	}
	if mimeType != "" {
		params.Set("mime_type", mimeType)
	}
	if qs := params.Encode(); qs != "" {
		return path + "?" + qs
	}
	return path
}
