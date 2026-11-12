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

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ghProxyURL is the only accepted GitHub mirror prefix; arbitrary GH_PROXY
// hosts are rejected so a stray environment variable cannot redirect native
// library downloads.
const ghProxyURL = "https://gh-proxy.com/"

const (
	downloadAttempts = 3
	connectTimeout   = 15 * time.Second
	idleReadTimeout  = 60 * time.Second
)

func resolveGHProxy(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.TrimRight(raw, "/") != strings.TrimRight(ghProxyURL, "/") {
		return "", fmt.Errorf("GH_PROXY must be empty or %q", ghProxyURL)
	}
	return ghProxyURL, nil
}

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: connectTimeout}).DialContext
	transport.ResponseHeaderTimeout = idleReadTimeout
	return &http.Client{Transport: transport}
}

func backoff(attempt int) {
	time.Sleep(time.Duration(attempt) * 2 * time.Second)
}

// statusError is a non-2xx response. Client errors are final; server errors
// and rate limiting are retried.
type statusError struct {
	url  string
	code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d", e.url, e.code)
}

func retryable(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		return se.code >= 500 || se.code == http.StatusTooManyRequests
	}
	return true
}

// download fetches url into dest atomically: the body streams into a temporary
// file next to dest, which replaces dest only after a complete, non-empty
// transfer. A failed download leaves any previous dest untouched.
func (d *downloader) download(url, dest string) error {
	var err error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		if err = d.downloadOnce(url, dest); err == nil || !retryable(err) {
			return err
		}
		if attempt < downloadAttempts {
			fmt.Fprintf(d.out, "  download failed (%v); retrying\n", err)
			d.retryDelay(attempt)
		}
	}
	return err
}

func (d *downloader) downloadOnce(url, dest string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancel a transfer that stalls for idleReadTimeout; each read re-arms it.
	idle := time.AfterFunc(idleReadTimeout, cancel)
	defer idle.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if d.hfToken != "" && strings.HasPrefix(url, d.hfEndpoint+"/") {
		req.Header.Set("Authorization", "Bearer "+d.hfToken)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return &statusError{url: url, code: resp.StatusCode}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	progress := &progressWriter{out: d.out, total: resp.ContentLength}
	body := &idleReader{r: resp.Body, timer: idle}
	n, copyErr := io.Copy(io.MultiWriter(tmp, progress), body)
	progress.finish()
	if closeErr := tmp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("GET %s: %w", url, copyErr)
	}
	if n == 0 {
		return fmt.Errorf("GET %s: empty response body", url)
	}
	return os.Rename(tmp.Name(), dest)
}

type idleReader struct {
	r     io.Reader
	timer *time.Timer
}

func (r *idleReader) Read(p []byte) (int, error) {
	r.timer.Reset(idleReadTimeout)
	return r.r.Read(p)
}

// progressWriter prints whole-percent progress when the size is known.
type progressWriter struct {
	out     io.Writer
	total   int64
	written int64
	shown   int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if p.total > 0 {
		if pct := p.written * 100 / p.total; pct != p.shown {
			p.shown = pct
			fmt.Fprintf(p.out, "\rProgress: %d%% (%d/%d bytes)", pct, p.written, p.total)
		}
	}
	return len(b), nil
}

func (p *progressWriter) finish() {
	if p.total > 0 {
		fmt.Fprintln(p.out)
	}
}

// fetchText returns a small text response, such as a .sha256 sidecar.
func (d *downloader) fetchText(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", &statusError{url: url, code: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return string(data), err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// nonEmptyFile reports whether path is a regular file with content.
func nonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
