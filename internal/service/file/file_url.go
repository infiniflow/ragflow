package file

import (
	"fmt"
	"net/url"
	"path/filepath"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
)

// UploadFromURL fetches a remote URL, saves the content to the tenant's root
// folder, and returns the file metadata map — mirroring Python
// FileService.upload_info(tenant_id, None, url).
//
// The remote fetch is SSRF-guarded (mirrors Python's assert_url_is_safe): the
// scheme must be http/https and every address the host resolves to must be
// globally routable; the validated IP is pinned for the actual connection — and
// re-validated on each redirect hop — to defeat DNS-rebinding. The HTTP client
// carries connect and overall timeouts, and the response body is bounded with
// truncation detection so an oversized file is rejected rather than silently
// clipped.
func (s *FileService) UploadFromURL(tenantID, rawURL string) (map[string]interface{}, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid or unsafe URL")
	}

	data, headers, finalURL, err := utility.FetchRemoteFileSafely(rawURL, maxRemoteFileSize)
	if err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	contentType := headers.Get("Content-Type")
	filename := normalizeRemoteUploadFilename(finalURL, contentType, data)
	if err := s.checkUploadInfoHealth(tenantID, filename); err != nil {
		return nil, err
	}
	filename, contentType, data = utility.NormalizeUploadInfoContent(filename, contentType, data)
	return s.storeUploadInfoBlob(storageImpl, tenantID, filename, contentType, data)
}

func normalizeRemoteUploadFilename(rawURL, contentType string, data []byte) string {
	parsed, err := url.Parse(rawURL)
	filename := "download"
	if err == nil {
		filename = sanitizeFilename(filepath.Base(parsed.Path))
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if ct == "application/pdf" || utility.BytesLooksLikePDF(data) {
		if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
			filename += ".pdf"
		}
	}
	return filename
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.TrimSpace(name)

	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		if r < 0x20 {
			return '_'
		}
		return r
	}, name)

	name = strings.Trim(name, ". ")

	if name == "" || name == "." || name == ".." {
		return "download"
	}
	if stem := strings.SplitN(strings.ToUpper(name), ".", 2)[0]; reservedDeviceNames[stem] {
		return "download"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
