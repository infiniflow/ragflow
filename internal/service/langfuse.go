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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

const (
	langfuseAuthCheckTimeout = 7 * time.Second
	// projectsPath is the Langfuse endpoint used for auth_check.
	langfuseProjectsPath = "/api/public/projects"
)

// LangfuseService manages per-tenant Langfuse credentials.
type LangfuseService struct {
	langfuseDAO *dao.LangfuseDAO
}

// NewLangfuseService creates a LangfuseService.
func NewLangfuseService() *LangfuseService {
	return &LangfuseService{langfuseDAO: dao.NewLangfuseDAO()}
}

// SetLangfuseAPIKeyRequest is the body for POST/PUT /langfuse/api-key.
type SetLangfuseAPIKeyRequest struct {
	SecretKey string `json:"secret_key" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
	Host      string `json:"host"       binding:"required"`
}

// LangfuseProject is one project entry returned by the Langfuse projects API.
type LangfuseProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SetAPIKey validates credentials against Langfuse, then upserts the record.
// Mirrors Python set_api_key.
func (s *LangfuseService) SetAPIKey(tenantID string, req *SetLangfuseAPIKeyRequest) (map[string]interface{}, error) {
	if strings.TrimSpace(req.SecretKey) == "" ||
		strings.TrimSpace(req.PublicKey) == "" ||
		strings.TrimSpace(req.Host) == "" {
		return nil, fmt.Errorf("Missing required fields")
	}

	if err := langfuseAuthCheck(req.PublicKey, req.SecretKey, req.Host); err != nil {
		return nil, fmt.Errorf("Invalid Langfuse keys")
	}

	// Atomic upsert: insert the credentials, or update them in place when a row
	// already exists for this tenant. Avoids the read-then-write race between
	// concurrent SetAPIKey calls for the same tenant.
	record := &entity.TenantLangfuse{
		TenantID:  tenantID,
		SecretKey: req.SecretKey,
		PublicKey: req.PublicKey,
		Host:      req.Host,
	}
	if err := s.langfuseDAO.UpsertByTenantID(record); err != nil {
		return nil, fmt.Errorf("failed to save Langfuse keys: %w", err)
	}

	return map[string]interface{}{
		"tenant_id":  tenantID,
		"public_key": req.PublicKey,
		"host":       req.Host,
	}, nil
}

// GetAPIKey retrieves the stored credentials and validates them.
// The secret_key is intentionally omitted from the response.
// Mirrors Python get_api_key.
func (s *LangfuseService) GetAPIKey(tenantID string) (map[string]interface{}, error) {
	entry, err := s.langfuseDAO.GetByTenantID(tenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // caller interprets nil as "no keys stored"
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if err := langfuseAuthCheck(entry.PublicKey, entry.SecretKey, entry.Host); err != nil {
		return nil, fmt.Errorf("Invalid Langfuse keys loaded")
	}

	projects, err := langfuseGetProjects(entry.PublicKey, entry.SecretKey, entry.Host)
	if err != nil {
		return nil, fmt.Errorf("Error from Langfuse: %w", err)
	}

	result := map[string]interface{}{
		"tenant_id":  entry.TenantID,
		"public_key": entry.PublicKey,
		"host":       entry.Host,
		// secret_key is intentionally excluded.
	}
	if len(projects) > 0 {
		result["project_id"] = projects[0].ID
		result["project_name"] = projects[0].Name
	}
	return result, nil
}

// DeleteAPIKey removes the stored Langfuse credentials for a tenant.
// Mirrors Python delete_api_key.
func (s *LangfuseService) DeleteAPIKey(tenantID string) (bool, error) {
	_, err := s.langfuseDAO.GetByTenantID(tenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil // caller interprets false as "no keys stored"
		}
		return false, fmt.Errorf("database error: %w", err)
	}

	if err := s.langfuseDAO.DeleteByTenantID(tenantID); err != nil {
		return false, fmt.Errorf("failed to delete Langfuse keys: %w", err)
	}
	return true, nil
}

// ── Langfuse API helpers ──────────────────────────────────────────────────────

// maxLangfuseResponseBytes bounds how much of a Langfuse response we read or
// drain, guarding against resource exhaustion from a hostile endpoint.
const maxLangfuseResponseBytes = 1 << 20 // 1 MB

// langfuseAuthCheck calls GET {host}/api/public/projects with Basic Auth.
// Returns nil on HTTP 2xx, error otherwise — mirrors Python langfuse.auth_check().
func langfuseAuthCheck(publicKey, secretKey, host string) error {
	if err := validateLangfuseHost(host); err != nil {
		return err
	}
	reqURL := strings.TrimRight(host, "/") + langfuseProjectsPath

	ctx, cancel := context.WithTimeout(context.Background(), langfuseAuthCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(publicKey, secretKey)

	resp, err := langfuseHTTPClient(langfuseAuthCheckTimeout).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Bound the drain so a hostile endpoint cannot stream an unbounded body.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxLangfuseResponseBytes))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Langfuse auth failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// langfuseGetProjects fetches the project list for the tenant.
func langfuseGetProjects(publicKey, secretKey, host string) ([]LangfuseProject, error) {
	if err := validateLangfuseHost(host); err != nil {
		return nil, err
	}
	reqURL := strings.TrimRight(host, "/") + langfuseProjectsPath

	ctx, cancel := context.WithTimeout(context.Background(), langfuseAuthCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(publicKey, secretKey)

	resp, err := langfuseHTTPClient(langfuseAuthCheckTimeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLangfuseResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Langfuse projects API returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data []LangfuseProject `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// validateLangfuseHost ensures the configured host is a well-formed http(s) URL.
// The user supplies this host and the server then issues requests to it, so it
// must be validated before use.
func validateLangfuseHost(host string) error {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(host), "/"))
	if err != nil {
		return fmt.Errorf("invalid Langfuse host")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("Langfuse host must use http or https")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("Langfuse host is missing a hostname")
	}
	return nil
}

// langfuseHTTPClient returns an HTTP client whose dialer resolves the target
// host, rejects any non-globally-routable IP, and pins the validated address —
// an SSRF guard (with DNS-rebinding protection) for the user-supplied Langfuse
// host. The IP check runs on every connection, so redirect hops are covered too.
func langfuseHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
				if err != nil {
					return nil, fmt.Errorf("could not resolve host %q: %w", host, err)
				}
				var pinned net.IP
				for _, ip := range ips {
					if !isGloballyRoutableIP(ip) {
						return nil, fmt.Errorf("Langfuse host resolves to a non-public address (%s)", ip)
					}
					if pinned == nil {
						pinned = ip
					}
				}
				if pinned == nil {
					return nil, fmt.Errorf("host %q resolved to no addresses", host)
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(pinned.String(), port))
			},
		},
	}
}

// isGloballyRoutableIP reports whether ip is a public, routable address. It
// rejects loopback, private, link-local, multicast, unspecified, and
// carrier-grade-NAT ranges. IPv4-mapped IPv6 addresses are handled by the
// stdlib predicates.
func isGloballyRoutableIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	// Carrier-grade NAT 100.64.0.0/10 (RFC 6598) — not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1]&0xc0 == 0x40 {
		return false
	}
	return true
}
