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

//go:build desktop

// Package desktop boots the in-process backends of the desktop build.
//
// The `desktop` build tag replaces every external service the deployed stack
// normally talks to:
//
//	Kvrocks      -> pebbis, served over loopback RESP from this process
//	NATS         -> nats-server with JetStream, in this process
//	MinIO / S3   -> versitygw POSIX gateway, in this process
//	Infinity     -> the embedded engine (embedded-ffi), linked into this process
//	MySQL        -> SQLite, one file in the data directory (see internal/dao)
//
// The entry points above are the only ones the desktop build has: it reports
// the SQLite path and the engine's loopback port the same way it reports the
// others, and internal/server/bootstrap pins both types so a configuration
// file or environment variable cannot point the process at a server.
//
// Every listener binds 127.0.0.1 on an ephemeral port, so a second instance and
// any service already using the conventional ports cannot collide, and nothing
// is reachable from another host.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	pebbis "github.com/infiniflow/pebbis"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/versity/versitygw/backend/meta"
	"github.com/versity/versitygw/backend/posix"
	"github.com/versity/versitygw/embedgw"
)

// Credentials are fixed rather than generated: every listener is loopback-only,
// so they exist to satisfy the S3 and RESP protocol handshakes, not to
// authenticate anyone. Rotating them would only make a saved data directory
// unusable after an upgrade.
const (
	// KvrocksPassword is the pebbis requirepass value.
	KvrocksPassword = "infini_rag_flow"
	// S3AccessKey and S3SecretKey are the versitygw root credentials.
	S3AccessKey = "ragflowembeddedacc"
	S3SecretKey = "ragflowembeddedsecret"
	// S3Region is what the gateway reports to S3 clients.
	S3Region = "us-east-1"
)

// Directory names under the data directory. They are part of the on-disk
// contract: an operator backing up the tree must keep them together.
const (
	pebbisDirName   = "pebbis"
	natsDirName     = "nats"
	objectsDirName  = "s3-objects"
	objectsMetaName = "s3-meta"
	objectsVersName = "s3-versions"
	infinityDirName = "infinity"
)

// legacyPebbisdDirName is what the RESP store's directory was called before it
// was named after the engine that serves it. Start renames it in place so an
// existing installation does not come up looking like a fresh one.
const legacyPebbisdDirName = "kvrocks"

// Runtime describes the endpoints the rest of the process must connect to, and
// owns the shutdown of everything Start launched.
//
// Close must be called on shutdown; it stops the backends in reverse start
// order so JetStream and Pebble both flush before the process exits.
type Runtime struct {
	// DataDir is the root the caller supplied; every backend owns a subdirectory.
	DataDir string

	// KvrocksAddr is the loopback "host:port" serving the RESP protocol.
	KvrocksAddr string
	// KvrocksPassword is the requirepass the RESP endpoint expects.
	KvrocksPassword string

	// NATSHost and NATSPort address the in-process JetStream server.
	NATSHost string
	NATSPort int

	// S3Endpoint is the loopback "host:port" of the S3 gateway; pass it as the
	// storage backend's host so the existing minio-go client needs no change.
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string

	// InfinityAddr is the loopback "host:port" the in-process document engine
	// serves its thrift API on. It is a port rather than a unix socket because
	// the engine has to run on Windows too, where a path-backed listener is not
	// an option.
	InfinityAddr string

	closers []func() error
}

// Close stops every backend. It reports the first failure but always attempts
// the rest: a stuck listener must not keep the data store open.
func (r *Runtime) Close() error {
	var firstErr error
	for i := len(r.closers) - 1; i >= 0; i-- {
		if err := r.closers[i](); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.closers = nil
	return firstErr
}

// Start creates the data layout and boots every backend, returning the
// endpoints to connect to. A failure part-way through stops whatever had
// already started, so a failed boot leaves no goroutines or open stores behind.
func Start(ctx context.Context, dataDir string) (*Runtime, error) {
	if dataDir == "" {
		return nil, errors.New("desktop: data directory is required")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("desktop: resolve data directory: %w", err)
	}
	if err := migrateLegacyLayout(abs); err != nil {
		return nil, err
	}
	if err := ensureLayout(abs); err != nil {
		return nil, err
	}
	rt := &Runtime{
		DataDir:         abs,
		KvrocksPassword: KvrocksPassword,
		S3AccessKey:     S3AccessKey,
		S3SecretKey:     S3SecretKey,
	}
	// A boot failure must not leak a half-started backend.
	fail := func(err error) (*Runtime, error) {
		_ = rt.Close()
		return nil, err
	}

	addr, stopKvrocks, err := startKvrocks(abs)
	if err != nil {
		return fail(err)
	}
	rt.KvrocksAddr = addr
	rt.closers = append(rt.closers, stopKvrocks)

	host, port, stopNATS, err := startNATS(ctx, abs)
	if err != nil {
		return fail(err)
	}
	rt.NATSHost, rt.NATSPort = host, port
	rt.closers = append(rt.closers, stopNATS)

	endpoint, stopS3, err := startS3(ctx, abs)
	if err != nil {
		return fail(err)
	}
	rt.S3Endpoint = endpoint
	rt.closers = append(rt.closers, stopS3)

	// The document engine goes last: it is the heaviest backend and the one
	// whose checkpoint has to finish before the stores it writes through go
	// away. Close runs the closers in reverse, so it therefore stops first.
	infinity, err := startEmbeddedInfinity(filepath.Join(abs, infinityDirName))
	if err != nil {
		return fail(err)
	}
	rt.InfinityAddr = infinity.Addr()
	rt.closers = append(rt.closers, infinity.Close)

	return rt, nil
}

// migrateLegacyLayout renames directories that were created under an older
// name. The RESP store used to live in <data>/kvrocks; leaving it there would
// make an existing installation come up with an empty key-value store.
func migrateLegacyLayout(dataDir string) error {
	legacy := filepath.Join(dataDir, legacyPebbisdDirName)
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}
	current := filepath.Join(dataDir, pebbisDirName)
	if _, err := os.Stat(current); err == nil {
		// Both exist: the new directory is the one in use, so leave the old one
		// untouched rather than merging two stores.
		return nil
	}
	if err := os.Rename(legacy, current); err != nil {
		return fmt.Errorf("desktop: rename %s to %s: %w", legacy, current, err)
	}
	return nil
}

// ensureLayout creates the per-backend directories. objects-meta and
// objects-versions must live outside the gateway root, so they are siblings of
// objects rather than children.
func ensureLayout(dataDir string) error {
	for _, name := range []string{
		pebbisDirName,
		natsDirName,
		objectsDirName,
		objectsMetaName,
		objectsVersName,
		infinityDirName,
	} {
		if err := os.MkdirAll(filepath.Join(dataDir, name), 0o755); err != nil {
			return fmt.Errorf("desktop: create %s: %w", name, err)
		}
	}
	return nil
}

// startKvrocks serves the pebbis store over loopback TCP.
//
// TCP rather than a unix socket is deliberate: pebbis chmods a unix socket to
// 0600, which a Windows host cannot do, and the socket path budget is far
// smaller there. A loopback port behaves the same on every platform and keeps
// the redis client configuration to a host:port pair.
func startKvrocks(dataDir string) (addr string, stop func() error, err error) {
	cfg := pebbis.DefaultOptions()
	cfg.RequirePass = KvrocksPassword

	emb, err := pebbis.OpenEmbedded(pebbis.EmbedOptions{
		Dir:     filepath.Join(dataDir, pebbisDirName),
		Network: "tcp",
		Addr:    "127.0.0.1:0",
		Config:  cfg,
	})
	if err != nil {
		return "", nil, fmt.Errorf("desktop: start pebbis: %w", err)
	}
	return emb.Addr(), emb.Close, nil
}

// startNATS boots a JetStream server writing to <data>/nats. Port -1 asks the
// server for an ephemeral port, which it reports back through Addr() once it is
// ready to accept connections.
func startNATS(ctx context.Context, dataDir string) (host string, port int, stop func() error, err error) {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  filepath.Join(dataDir, natsDirName),
		NoSigs:    true,
		NoLog:     true,
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		return "", 0, nil, fmt.Errorf("desktop: configure nats: %w", err)
	}
	go srv.Start()

	// ReadyForConnections also pumps the server's internal goroutines, so it
	// must be called from the process that started it.
	if !srv.ReadyForConnections(10 * time.Second) {
		srv.Shutdown()
		return "", 0, nil, errors.New("desktop: nats server did not become ready")
	}
	tcp, ok := srv.Addr().(*net.TCPAddr)
	if !ok {
		srv.Shutdown()
		return "", 0, nil, fmt.Errorf("desktop: unexpected nats listener %v", srv.Addr())
	}

	stopFunc := func() error {
		srv.Shutdown()
		srv.WaitForShutdown()
		return nil
	}
	// Shut the server down when the caller's context ends, so a cancelled boot
	// cannot leave a JetStream directory locked.
	go func() {
		<-ctx.Done()
		_ = stopFunc()
	}()
	return "127.0.0.1", tcp.Port, stopFunc, nil
}

// startS3 boots the versitygw POSIX gateway.
//
// The layout is chosen for portability, not throughput:
//   - sidecar metadata instead of xattrs, because a Windows or macOS host does
//     not share Linux's user.* namespace and the gateway fails its startup
//     probe when xattrs are unavailable;
//   - local advisory locks, because the only writer is this process;
//   - absolute paths, because the gateway would otherwise chdir the whole
//     server into the object root.
func startS3(ctx context.Context, dataDir string) (endpoint string, stop func() error, err error) {
	root := filepath.Join(dataDir, objectsDirName)
	sidecar := filepath.Join(dataDir, objectsMetaName)
	versioning := filepath.Join(dataDir, objectsVersName)

	ms, err := meta.NewSideCar(sidecar)
	if err != nil {
		return "", nil, fmt.Errorf("desktop: init s3 sidecar metadata: %w", err)
	}
	be, err := posix.New(root, ms, posix.PosixOpts{
		AbsolutePaths:  true,
		SideCarDir:     sidecar,
		VersioningDir:  versioning,
		ObjectLockMode: posix.ObjectLockModeLocal,
		Concurrency:    256,
		// A zero threshold rejects every CopyObject ("larger than the maximum
		// allowable size for a copy source: 0"), and the storage layer's Move
		// is copy-then-delete. 5 GiB is the S3 limit for a single copy.
		CopyObjectThreshold: 5 << 30,
	})
	if err != nil {
		return "", nil, fmt.Errorf("desktop: init s3 backend: %w", err)
	}

	addr, err := freeLoopbackAddr()
	if err != nil {
		return "", nil, err
	}

	gwCtx, cancel := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() {
		runErr <- embedgw.RunVersityGW(gwCtx, be, &embedgw.Config{
			RootUserAccess:    S3AccessKey,
			RootUserSecret:    S3SecretKey,
			Region:            S3Region,
			Ports:             []string{addr},
			MaxConnections:    256,
			MaxRequests:       256,
			MultipartMaxParts: 10000,
			Quiet:             true,
		})
	}()

	stopFunc := func() error {
		cancel()
		select {
		case err := <-runErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			return nil
		case <-time.After(30 * time.Second):
			return errors.New("desktop: s3 gateway did not stop within 30s")
		}
	}

	if err := waitForHTTP(gwCtx, addr, runErr, 30*time.Second); err != nil {
		_ = stopFunc()
		return "", nil, err
	}
	return addr, stopFunc, nil
}

// freeLoopbackAddr reserves an ephemeral loopback port and releases it.
//
// embedgw takes listen addresses as strings and never hands back the listener it
// creates, so the port has to be picked before the call. The window between the
// probe and the gateway's own bind is tiny and only matters when something else
// steals the port in that instant; that surfaces as a gateway start failure
// rather than a wrong address.
func freeLoopbackAddr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("desktop: reserve s3 port: %w", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", fmt.Errorf("desktop: release s3 port: %w", err)
	}
	return addr, nil
}

// waitForHTTP waits until addr answers an HTTP request or the gateway dies.
func waitForHTTP(ctx context.Context, addr string, runErr <-chan error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &httpClient{timeout: 500 * time.Millisecond}
	for {
		select {
		case err := <-runErr:
			if err == nil {
				err = errors.New("gateway stopped")
			}
			return fmt.Errorf("desktop: s3 gateway exited during startup: %w", err)
		case <-ctx.Done():
			return fmt.Errorf("desktop: s3 gateway startup cancelled: %w", ctx.Err())
		default:
		}
		if client.reachable(ctx, addr) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("desktop: s3 gateway did not answer on %s within %s", addr, timeout)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("desktop: s3 gateway startup cancelled: %w", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// httpClient is a minimal liveness probe: any HTTP status means the listener is
// up and parsing requests, which is all startup waits for.
type httpClient struct{ timeout time.Duration }

func (c *httpClient) reachable(ctx context.Context, addr string) bool {
	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(c.timeout))
	// Signed request not needed: the gateway answers 403 to an unsigned
	// ListBuckets, and a 403 still proves the S3 front end is serving.
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n")); err != nil {
		return false
	}
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}
	return n > 0 && string(buf[:3]) == "HTT"
}
