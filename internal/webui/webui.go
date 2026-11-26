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

// Package webui serves the built web frontend to the desktop shell.
//
// The desktop app bundles the frontend into the Go binary and serves it from one
// loopback origin, so everything the frontend does is same-origin:
//
//	/api/v1/admin/...  -> the admin server (a second listener in this process)
//	/api/..., /v1/...  -> the API server (the main listener)
//	everything else    -> the embedded static bundle, falling back to index.html
//
// Serving from one origin is what keeps the frontend unchanged: it addresses the
// API with relative paths, and the two servers keep running on their own ports.
package webui

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"
)

// ErrNoAssets means the binary was built without a frontend bundle. Only the
// desktop build compiles one in (see assets_desktop.go).
var ErrNoAssets = errors.New("this build has no bundled web frontend")

// Options are the loopback addresses the bundled frontend talks to.
type Options struct {
	// APIAddr is the host:port of the API server, e.g. 127.0.0.1:9380.
	APIAddr string
	// AdminAddr is the host:port of the admin server, e.g. 127.0.0.1:9381.
	AdminAddr string
}

// New builds the handler: proxied API and admin, static bundle for the rest.
func New(opts Options) (http.Handler, error) {
	if !Available() {
		return nil, ErrNoAssets
	}
	static, err := newBundle()
	if err != nil {
		return nil, err
	}
	api := reverseProxy(opts.APIAddr)
	admin := reverseProxy(opts.AdminAddr)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case isAdminPath(p):
			admin.ServeHTTP(w, r)
		case isAPIPath(p):
			api.ServeHTTP(w, r)
		default:
			static.ServeHTTP(w, r)
		}
	}), nil
}

// Listen serves the UI on addr. A port of 0 asks the OS for a free one, which is
// what the desktop shell wants: several instances must not fight over a fixed
// port. The returned URL is what a window should load.
func Listen(addr string, opts Options) (url string, shutdown func(), err error) {
	handler, err := New(opts)
	if err != nil {
		return "", nil, err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			// A dead UI listener leaves the window blank; nothing sensible to do
			// about it here, so let the shutdown path report it.
			fmt.Printf("web UI server stopped: %v\n", serveErr)
		}
	}()
	uiURL := fmt.Sprintf("http://%s/", listener.Addr().String())
	return uiURL, func() { _ = server.Close() }, nil
}

func isAdminPath(p string) bool {
	return p == "/api/v1/admin" || strings.HasPrefix(p, "/api/v1/admin/")
}

func isAPIPath(p string) bool {
	return p == "/api" || strings.HasPrefix(p, "/api/") || p == "/v1" || strings.HasPrefix(p, "/v1/")
}

// reverseProxy forwards one path family to the server that owns it. The proxy
// keeps WebSocket upgrades working (httputil.ReverseProxy handles them), which
// the frontend uses for streaming answers.
func reverseProxy(addr string) *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: addr}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// FlushInterval -1 streams every write immediately: answers arrive token by
	// token, and buffering would defeat that.
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, fmt.Sprintf("%s is not reachable: %v", addr, err), http.StatusBadGateway)
	}
	return proxy
}

// bundle serves the embedded frontend. Unknown paths fall back to index.html so
// client-side routes survive a reload.
type bundle struct {
	files fs.FS
	index []byte
}

func newBundle() (*bundle, error) {
	root, err := assets()
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read bundled index.html: %w", err)
	}
	return &bundle{files: root, index: index}, nil
}

func (b *bundle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		b.writeIndex(w, r)
		return
	}
	data, err := fs.ReadFile(b.files, name)
	if err != nil {
		// A directory, a missing file or a client-side route: the app decides.
		b.writeIndex(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", cacheControlFor(name))
	_, _ = w.Write(data)
}

func (b *bundle) writeIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(string(b.index)))
}

// cacheControlFor marks content-hashed asset paths immutable and everything else
// revalidated: index.html and conf.json must never be served from a stale cache
// after an upgrade.
func cacheControlFor(name string) string {
	for _, prefix := range []string{"entry/", "chunk/", "assets/", "monaco/", "pdfjs-dist/"} {
		if strings.HasPrefix(name, prefix) {
			return "public, max-age=31536000, immutable"
		}
	}
	return "no-cache"
}
