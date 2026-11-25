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

// boxd runs each CodeExec call in a disposable boxd machine: CreateInstance
// creates an isolated machine, closes its outbound network unless
// allow_internet is set and makes a per-instance working directory,
// ExecuteCode uploads the shared RAGFlow wrapper and runs it with `python3`
// or `node`, and DestroyInstance deletes the machine.
//
// It speaks the boxd public gRPC API (https://docs.boxd.sh/reference/grpc-api)
// through the client in boxdpb, generated from the public proto. The API key
// is exchanged for a short-lived token at the console's /api/v1/auth/token
// endpoint and the token is renewed before it expires. Artifacts are
// collected with the same allowlist and result shape as the local, ssh,
// tenki, ucloud and kosa8 providers.

package sandbox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"ragflow/internal/agent/sandbox/boxdpb"
	"ragflow/internal/common"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	boxdDefaultBaseURL       = "https://boxd.sh:9443"
	boxdDefaultNamePrefix    = "ragflow-sandbox-"
	boxdAPIPort              = "9443"
	boxdTokenPath            = "/api/v1/auth/token"
	boxdHome                 = "/home/boxd"
	boxdDefaultTimeout       = 30
	boxdDefaultMaxLifetime   = 3600
	boxdDefaultMaxOutput     = 1 << 20
	boxdDefaultMaxArtifacts  = 20
	boxdDefaultArtifactBytes = 10 << 20
	boxdMaxArtifactDepth     = 16
	// boxdMaxListing bounds the artifact listing a run can produce.
	boxdMaxListing = 1 << 20
	// boxdGiBPerVCPU is the memory of every boxd size: 1x4, 2x8, 4x16, 8x32.
	boxdGiBPerVCPU   = 4
	boxdStartTimeout = 2 * time.Minute
	boxdStartPoll    = 500 * time.Millisecond
	// boxdCallTimeout bounds every call that is not a code execution.
	boxdCallTimeout = time.Minute
	// boxdExecGrace is how long past its timeout a run is waited for
	// before the call is cancelled, which also ends the command.
	boxdExecGrace = 30 * time.Second
	// boxdKillAfter is how long `timeout` waits after SIGTERM before
	// it sends SIGKILL.
	boxdKillAfter = 5
	// A token is renewed this long before it expires.
	boxdTokenRenew = 5 * time.Minute
	// A delete that fails with a transient error is tried
	// boxdDeleteAttempts times in all, with jittered exponential backoff.
	boxdDeleteAttempts   = 3
	boxdDeleteBackoff    = 500 * time.Millisecond
	boxdDeleteMaxBackoff = 3 * time.Second
	boxdUploadChunk      = 1 << 20
)

var (
	boxdRegionPattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	boxdNamePrefixPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	boxdLabelKeyPattern   = regexp.MustCompile(`^[a-z0-9]([a-z0-9._/-]{0,61}[a-z0-9])?$`)
	boxdSizes             = []int{0, 1, 2, 4, 8}
)

type boxdInstance struct {
	machineID string
	workDir   string
}

// BoxdProvider executes code in disposable boxd machines.
type BoxdProvider struct {
	apiKey           string
	baseURL          string
	region           string
	org              string
	namePrefix       string
	labels           string
	vcpu             int
	allowInternet    bool
	timeoutSec       int
	maxLifetimeSec   int
	maxOutputBytes   int
	maxArtifacts     int
	maxArtifactBytes int

	startTimeout  time.Duration
	startPoll     time.Duration
	execGrace     time.Duration
	deleteBackoff time.Duration
	// dialOptions are added to the gRPC dial options (tests use them to
	// dial an in-memory server).
	dialOptions []grpc.DialOption

	mu          sync.RWMutex
	initialized bool
	conn        *grpc.ClientConn
	api         boxdpb.BoxdApiClient
	exchangeURL string
	httpClient  *http.Client
	labelSet    map[string]string
	instances   map[string]boxdInstance

	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time
}

type boxdExecResult struct {
	stdout string
	stderr string
	exit   int
}

func newBoxdProviderFromEnv() *BoxdProvider {
	return newBoxdProviderFromConfig(boxdConfigFromEnv())
}

// boxdConfigFromEnv builds a config map from the BOXD_* env vars,
// mirroring the admin-panel settings JSON shape.
func boxdConfigFromEnv() map[string]any {
	return map[string]any{
		"api_key":        common.GetEnv(common.EnvBoxdAPIKey),
		"base_url":       common.GetEnv(common.EnvBoxdBaseURL),
		"region":         common.GetEnv(common.EnvBoxdRegion),
		"org":            common.GetEnv(common.EnvBoxdOrg),
		"timeout":        common.GetEnv(common.EnvBoxdTimeout),
		"allow_internet": common.GetEnv(common.EnvBoxdAllowInternet),
	}
}

func newBoxdProviderFromConfig(cfg map[string]any) *BoxdProvider {
	namePrefix := strings.TrimSpace(configString(cfg, "name_prefix"))
	if namePrefix == "" {
		namePrefix = boxdDefaultNamePrefix
	}
	return &BoxdProvider{
		apiKey:     strings.TrimSpace(configString(cfg, "api_key")),
		baseURL:    strings.TrimRight(strings.TrimSpace(configString(cfg, "base_url")), "/"),
		region:     strings.ToLower(strings.TrimSpace(configString(cfg, "region"))),
		org:        strings.TrimSpace(configString(cfg, "org")),
		namePrefix: namePrefix,
		labels:     configString(cfg, "labels"),
		vcpu:       configInt(cfg, "vcpu", 0),
		// Outbound network is opt-in, as for tenki, ucloud and kosa8:
		// sandboxed code reaches nothing unless allow_internet is "true".
		allowInternet:    strings.EqualFold(configString(cfg, "allow_internet"), "true"),
		timeoutSec:       configInt(cfg, "timeout", boxdDefaultTimeout),
		maxLifetimeSec:   configInt(cfg, "max_lifetime", boxdDefaultMaxLifetime),
		maxOutputBytes:   configInt(cfg, "max_output_bytes", boxdDefaultMaxOutput),
		maxArtifacts:     configInt(cfg, "max_artifacts", boxdDefaultMaxArtifacts),
		maxArtifactBytes: configInt(cfg, "max_artifact_bytes", boxdDefaultArtifactBytes),
		startTimeout:     boxdStartTimeout,
		startPoll:        boxdStartPoll,
		execGrace:        boxdExecGrace,
		deleteBackoff:    boxdDeleteBackoff,
		instances:        make(map[string]boxdInstance),
	}
}

// ProviderType returns ProviderBoxd.
func (p *BoxdProvider) ProviderType() ProviderType { return ProviderBoxd }

// Initialize checks the configuration and opens the gRPC client. The API
// key comes from the admin config or BOXD_API_KEY.
func (p *BoxdProvider) Initialize(context.Context) error {
	if p.apiKey == "" {
		p.apiKey = strings.TrimSpace(common.GetEnv(common.EnvBoxdAPIKey))
	}
	if p.apiKey == "" {
		return errors.New("boxd: API key is required (set it in Admin > Sandbox Settings or BOXD_API_KEY)")
	}
	if p.baseURL == "" {
		p.baseURL = strings.TrimRight(common.GetEnv(common.EnvBoxdBaseURL), "/")
	}
	if p.baseURL == "" {
		p.baseURL = boxdDefaultBaseURL
	}
	if p.timeoutSec <= 0 || p.maxLifetimeSec < 60 || p.maxOutputBytes <= 0 || p.maxArtifactBytes <= 0 || p.maxArtifacts < 0 {
		return errors.New("boxd: timeout and size limits must be positive, max_lifetime at least 60 seconds, and max_artifacts non-negative")
	}
	if !slices.Contains(boxdSizes, p.vcpu) {
		return fmt.Errorf("boxd: vcpu must be 0 (the org's default), 1, 2, 4 or 8, not %d", p.vcpu)
	}
	if !boxdNamePrefixPattern.MatchString(p.namePrefix) {
		return fmt.Errorf("boxd: name_prefix %q must be up to 40 lowercase letters, digits and hyphens", p.namePrefix)
	}
	labels, err := parseBoxdLabels(p.labels)
	if err != nil {
		return err
	}
	target, secure, exchangeURL, err := boxdEndpoints(p.baseURL, p.region)
	if err != nil {
		return err
	}
	transport := insecure.NewCredentials()
	if secure {
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	opts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(transport),
		grpc.WithPerRPCCredentials(boxdTokenCredentials{provider: p, secure: secure}),
		grpc.WithUserAgent("ragflow-boxd"),
	}, p.dialOptions...)
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return fmt.Errorf("boxd: open API client: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.conn = conn
	p.api = boxdpb.NewBoxdApiClient(conn)
	p.exchangeURL = exchangeURL
	p.httpClient = &http.Client{Timeout: 30 * time.Second}
	p.labelSet = labels
	p.initialized = true
	return nil
}

// SupportedLanguages returns the languages the boxd image runs.
func (p *BoxdProvider) SupportedLanguages() []string {
	return []string{"python", "javascript"}
}

// CreateInstance creates an isolated boxd machine, waits until it runs
// commands, closes its outbound network unless allow_internet is set and
// makes the per-instance working directory. A machine that fails to get
// ready is deleted.
func (p *BoxdProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, errors.New("boxd: provider not initialized")
	}
	language := normalizeLanguage(template)
	if language == "" {
		return nil, fmt.Errorf("boxd: unsupported language %q", template)
	}
	name := p.namePrefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	req := &boxdpb.CreateVmRequest{
		Name:     name,
		Org:      p.org,
		Isolated: true,
		Labels:   p.machineLabels(),
		Config: &boxdpb.VmConfig{
			Vcpu:        uint32(p.vcpu),
			MemoryBytes: uint64(p.vcpu) * boxdGiBPerVCPU << 30,
			// A run that prints nothing for a while must not be put
			// to sleep; the machine is deleted when the run ends.
			Srf: &boxdpb.SrfConfig{AutoSuspendTimeoutSecs: proto.Uint32(0)},
		},
		// The server-side backstop that deletes a machine whose
		// DestroyInstance never ran.
		DeleteAfterSecs: proto.Uint64(uint64(p.maxLifetimeSec)),
	}
	callCtx, cancel := context.WithTimeout(ctx, boxdCallTimeout)
	created, err := p.api.CreateVm(callCtx, req)
	cancel()
	if err != nil {
		// The machine may exist although the answer was lost: delete it
		// by name. A name that is not there counts as deleted.
		if boxdMaybeCreated(err) {
			p.cleanup(ctx, name)
		}
		return nil, boxdCallError("create machine", err)
	}
	machineID := created.GetVmId()
	if machineID == "" {
		p.cleanup(ctx, name)
		return nil, errors.New("boxd: create machine: answer has no machine id")
	}
	workDir := path.Join(boxdHome, "ragflow-codeexec-"+uuid.NewString())
	if err := p.prepare(ctx, machineID, created.GetStatus(), workDir); err != nil {
		p.cleanup(ctx, machineID)
		return nil, err
	}
	p.mu.Lock()
	p.instances[machineID] = boxdInstance{machineID: machineID, workDir: workDir}
	p.mu.Unlock()
	return &SandboxInstance{
		InstanceID: machineID,
		Provider:   ProviderBoxd,
		Status:     "running",
		Metadata: map[string]any{
			"language":        language,
			"machine_id":      machineID,
			"machine_name":    name,
			"remote_work_dir": workDir,
			"max_lifetime":    p.maxLifetimeSec,
		},
	}, nil
}

// prepare waits until the machine runs commands, applies its egress
// policy and makes the per-instance artifacts directory.
func (p *BoxdProvider) prepare(ctx context.Context, machineID, state, workDir string) error {
	ctx, cancel := context.WithTimeout(ctx, p.startTimeout)
	defer cancel()
	for state != "running" {
		switch state {
		case "failed", "stopped", "destroyed":
			return fmt.Errorf("boxd: machine %s did not start: it is %s", machineID, state)
		}
		if err := boxdWait(ctx, p.startPoll); err != nil {
			return fmt.Errorf("boxd: machine %s still starting after %s", machineID, p.startTimeout)
		}
		got, err := p.api.GetVm(ctx, &boxdpb.GetVmRequest{VmId: machineID})
		// NotFound is polled again too, until the start deadline.
		if err != nil && !boxdTransient(err) && status.Code(err) != codes.NotFound {
			return boxdCallError("wait for machine", err)
		}
		state = got.GetStatus()
	}
	if !p.allowInternet {
		res, err := p.api.SetVmEgressAllow(ctx, &boxdpb.SetVmEgressAllowRequest{Vm: machineID, Entries: []string{"none"}})
		if err != nil {
			return boxdCallError("close outbound network", err)
		}
		if !slices.Equal(res.GetEntries(), []string{"none"}) {
			return fmt.Errorf("boxd: close outbound network: the machine allows %v", res.GetEntries())
		}
	}
	// Make the working directory, trying again on transient errors and
	// NotFound until the start deadline.
	mkdir := "mkdir -p " + shellQuoteBoxd(path.Join(workDir, "artifacts"))
	for {
		res, err := p.exec(ctx, machineID, mkdir, boxdMaxListing)
		if err == nil {
			if res.exit != 0 {
				return fmt.Errorf("boxd: create workspace: exit %d: %s", res.exit, strings.TrimSpace(res.stderr))
			}
			return nil
		}
		if !boxdTransient(err) && status.Code(err) != codes.NotFound {
			return boxdCallError("create workspace", err)
		}
		if boxdWait(ctx, p.startPoll) != nil {
			return fmt.Errorf("boxd: machine %s does not run commands after %s: %w", machineID, p.startTimeout, err)
		}
	}
}

// ExecuteCode uploads the shared RAGFlow wrapper into the machine and runs
// it in the instance's working directory under `timeout`.
func (p *BoxdProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, errors.New("boxd: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, errors.New("boxd: instance id required")
	}
	p.mu.RLock()
	instance, ok := p.instances[inst.InstanceID]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("boxd: unknown instance %q", inst.InstanceID)
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("boxd: unsupported language %q", language)
	}
	if timeoutSec == 0 {
		timeoutSec = p.timeoutSec
	}
	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	timeout = min(timeout, p.timeoutSec)
	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}
	var scriptName, script, executable string
	if lang == "python" {
		scriptName, script, executable = "main.py", BuildPythonWrapper(code, argsJSON), "python3"
	} else {
		scriptName, script, executable = "main.js", BuildJavaScriptWrapper(code, argsJSON), "node"
	}
	scriptPath := path.Join(instance.workDir, scriptName)
	if err := p.upload(ctx, instance.machineID, scriptPath, []byte(script)); err != nil {
		return nil, boxdCallError("write script", err)
	}

	command := fmt.Sprintf("cd %s && exec timeout -k %d %d %s %s",
		shellQuoteBoxd(instance.workDir), boxdKillAfter, timeout, executable, scriptName)
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second+p.execGrace)
	defer cancel()
	started := time.Now()
	res, err := p.exec(callCtx, instance.machineID, command, p.maxOutputBytes)
	elapsed := time.Since(started)
	if err != nil {
		if ctx.Err() == nil && callCtx.Err() != nil {
			return nil, fmt.Errorf("boxd: execution timed out after %d seconds", timeout)
		}
		var limitErr boxdOutputLimitError
		if errors.As(err, &limitErr) {
			return nil, err
		}
		return nil, boxdCallError("execute code", err)
	}
	// `timeout` exits 124 when it stopped the command and 137 when it
	// had to kill it.
	if (res.exit == 124 || res.exit == 137) && elapsed >= time.Duration(timeout)*time.Second {
		return nil, fmt.Errorf("boxd: execution timed out after %d seconds", timeout)
	}
	stdout, structured := ExtractStructuredResult(res.stdout)
	result := &ExecutionResult{
		Stdout:        stdout,
		Stderr:        res.stderr,
		ExitCode:      res.exit,
		ExecutionTime: elapsed.Seconds(),
		Metadata: map[string]any{
			"instance_id":       inst.InstanceID,
			"machine_id":        instance.machineID,
			"language":          lang,
			"script_path":       scriptPath,
			"remote_work_dir":   instance.workDir,
			"timeout":           timeout,
			"structured_result": structured,
		},
	}
	artifacts, err := p.collectArtifacts(ctx, instance.machineID, path.Join(instance.workDir, "artifacts"))
	if err != nil {
		return nil, err
	}
	result.Metadata["artifacts"] = artifacts
	return result, nil
}

// DestroyInstance deletes the machine. A machine that is already gone
// counts as deleted, so the call is idempotent. The instance is forgotten
// only once the machine is deleted: after a failed delete a later
// DestroyInstance tries again.
func (p *BoxdProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return errors.New("boxd: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return errors.New("boxd: instance id required")
	}
	p.mu.RLock()
	instance, ok := p.instances[inst.InstanceID]
	p.mu.RUnlock()
	if !ok {
		return nil
	}
	if err := p.deleteMachine(ctx, instance.machineID); err != nil {
		return boxdCallError("delete machine "+instance.machineID, err)
	}
	p.mu.Lock()
	delete(p.instances, inst.InstanceID)
	p.mu.Unlock()
	return nil
}

// HealthCheck calls the API with the configured key.
func (p *BoxdProvider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized() {
		return errors.New("boxd: provider not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, boxdCallTimeout)
	defer cancel()
	if _, err := p.api.Whoami(ctx, &boxdpb.WhoamiRequest{}); err != nil {
		return boxdCallError("health check", err)
	}
	return nil
}

// cleanup deletes a machine after a failed CreateInstance. It runs even
// when ctx was cancelled.
func (p *BoxdProvider) cleanup(ctx context.Context, machine string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	_ = p.deleteMachine(cleanupCtx, machine)
}

// deleteMachine deletes a machine by id or name, retrying transient
// errors. A machine that is already gone counts as deleted.
func (p *BoxdProvider) deleteMachine(ctx context.Context, machine string) error {
	for attempt := 1; ; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, boxdCallTimeout)
		_, err := p.api.DestroyVm(callCtx, &boxdpb.DestroyVmRequest{VmId: machine})
		cancel()
		if err == nil || status.Code(err) == codes.NotFound {
			return nil
		}
		if attempt >= boxdDeleteAttempts || !boxdTransient(err) || ctx.Err() != nil {
			return err
		}
		sleepCtx(ctx, backoff(p.deleteBackoff, boxdDeleteMaxBackoff, attempt))
	}
}

// exec runs a shell command and collects its output. More than maxOutput
// bytes of stdout and stderr together end the call, which also ends the
// command.
func (p *BoxdProvider) exec(ctx context.Context, machineID, command string, maxOutput int) (*boxdExecResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := p.api.Exec(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&boxdpb.ExecChunk{VmId: machineID, Command: command}); err != nil {
		if _, recvErr := stream.Recv(); recvErr != nil && recvErr != io.EOF {
			return nil, recvErr
		}
		return nil, err
	}
	// No stdin: the command reads end of file.
	if err := stream.CloseSend(); err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	exit, ended := 0, false
	// The stream ends with a chunk that carries the exit status and no
	// output. Output chunks never count as the end.
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(chunk.GetData()) == 0 || chunk.GetExitCode() != 0 {
			exit, ended = int(chunk.GetExitCode()), true
		}
		if len(chunk.GetData()) > 0 {
			if chunk.GetIsStderr() {
				stderr.Write(chunk.GetData())
			} else {
				stdout.Write(chunk.GetData())
			}
			if stdout.Len()+stderr.Len() > maxOutput {
				return nil, boxdOutputLimitError{limit: maxOutput}
			}
		}
	}
	if !ended {
		return nil, errors.New("boxd: command ended without an exit status")
	}
	return &boxdExecResult{stdout: stdout.String(), stderr: stderr.String(), exit: exit}, nil
}

// upload writes content to filePath in the machine.
func (p *BoxdProvider) upload(ctx context.Context, machineID, filePath string, content []byte) error {
	ctx, cancel := context.WithTimeout(ctx, boxdCallTimeout)
	defer cancel()
	stream, err := p.api.UploadFileStream(ctx)
	if err != nil {
		return err
	}
	first := &boxdpb.UploadFileChunk{VmId: machineID, Path: filePath, TotalSize: uint64(len(content))}
	for offset := 0; offset == 0 || offset < len(content); offset += boxdUploadChunk {
		chunk := first
		if offset > 0 {
			chunk = &boxdpb.UploadFileChunk{}
		}
		chunk.Data = content[offset:min(offset+boxdUploadChunk, len(content))]
		if err := stream.Send(chunk); err != nil {
			if _, recvErr := stream.CloseAndRecv(); recvErr != nil {
				return recvErr
			}
			return err
		}
	}
	res, err := stream.CloseAndRecv()
	if err != nil {
		return err
	}
	if written := res.GetBytesWritten(); written != 0 && written != uint64(len(content)) {
		return fmt.Errorf("wrote %d of %d bytes", written, len(content))
	}
	return nil
}

// download reads a file of at most limit bytes from the machine.
func (p *BoxdProvider) download(ctx context.Context, machineID, filePath string, limit int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, boxdCallTimeout)
	defer cancel()
	stream, err := p.api.DownloadFileStream(ctx, &boxdpb.DownloadFileRequest{VmId: machineID, Path: filePath})
	if err != nil {
		return nil, err
	}
	var data bytes.Buffer
	var total uint64
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk.GetTotalSize() > 0 {
			total = chunk.GetTotalSize()
			if total > uint64(limit) {
				return nil, fmt.Errorf("file is %d bytes, over the %d byte limit", total, limit)
			}
		}
		data.Write(chunk.GetData())
		if data.Len() > limit {
			return nil, fmt.Errorf("file is over the %d byte limit", limit)
		}
	}
	if total != 0 && uint64(data.Len()) != total {
		return nil, fmt.Errorf("download truncated: got %d of %d bytes", data.Len(), total)
	}
	return data.Bytes(), nil
}

type boxdArtifactEntry struct {
	kind string
	size int64
	path string
}

// collectArtifacts lists the artifacts directory with one command and
// downloads each allowed file.
func (p *BoxdProvider) collectArtifacts(ctx context.Context, machineID, dir string) ([]map[string]any, error) {
	listCtx, cancel := context.WithTimeout(ctx, boxdCallTimeout)
	defer cancel()
	listing := "cd " + shellQuoteBoxd(dir) + ` 2>/dev/null || exit 0; find . -mindepth 1 -printf '%y\t%s\t%P\0'`
	res, err := p.exec(listCtx, machineID, listing, boxdMaxListing)
	var limitErr boxdOutputLimitError
	if errors.As(err, &limitErr) {
		return nil, fmt.Errorf("boxd: the artifact listing exceeded %d bytes", boxdMaxListing)
	}
	if err != nil {
		return nil, boxdCallError("list artifacts", err)
	}
	if res.exit != 0 {
		return nil, fmt.Errorf("boxd: list artifacts: exit %d: %s", res.exit, strings.TrimSpace(res.stderr))
	}
	entries, err := parseBoxdListing(res.stdout)
	if err != nil {
		return nil, err
	}
	artifacts := make([]map[string]any, 0)
	for _, entry := range entries {
		if strings.Count(entry.path, "/") > boxdMaxArtifactDepth {
			return nil, fmt.Errorf("boxd: artifact directory nesting exceeds %d levels: %s", boxdMaxArtifactDepth, entry.path)
		}
		switch entry.kind {
		case "d":
			continue
		case "f":
		default:
			return nil, fmt.Errorf("boxd: artifact symlinks and special files are not allowed: %s", entry.path)
		}
		if len(artifacts) >= p.maxArtifacts {
			return nil, fmt.Errorf("boxd: execution produced more than %d artifacts", p.maxArtifacts)
		}
		if entry.size > int64(p.maxArtifactBytes) {
			return nil, fmt.Errorf("boxd: artifact exceeds %d bytes: %s", p.maxArtifactBytes, entry.path)
		}
		ext := strings.ToLower(path.Ext(entry.path))
		if _, ok := allowedArtifactExts[ext]; !ok {
			return nil, fmt.Errorf("boxd: unsupported artifact type: %s", entry.path)
		}
		content, err := p.download(ctx, machineID, path.Join(dir, entry.path), p.maxArtifactBytes)
		if err != nil {
			return nil, boxdCallError("read artifact "+entry.path, err)
		}
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		artifacts = append(artifacts, map[string]any{
			"name":        entry.path,
			"content_b64": base64.StdEncoding.EncodeToString(content),
			"mime_type":   mimeType,
			"size":        int64(len(content)),
		})
	}
	return artifacts, nil
}

// parseBoxdListing reads `find -printf '%y\t%s\t%P\0'` output and orders
// it depth-first by name, as the other providers walk the directory.
func parseBoxdListing(out string) ([]boxdArtifactEntry, error) {
	entries := make([]boxdArtifactEntry, 0)
	for _, record := range strings.Split(out, "\x00") {
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\t", 3)
		if len(fields) != 3 || fields[2] == "" {
			return nil, fmt.Errorf("boxd: bad artifact listing entry %q", record)
		}
		size, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("boxd: bad artifact listing entry %q", record)
		}
		clean := path.Clean(fields[2])
		if clean != fields[2] || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("boxd: invalid artifact path: %s", fields[2])
		}
		entries = append(entries, boxdArtifactEntry{kind: fields[0], size: size, path: clean})
	}
	slices.SortFunc(entries, func(a, b boxdArtifactEntry) int {
		return slices.Compare(strings.Split(a.path, "/"), strings.Split(b.path, "/"))
	})
	return entries, nil
}

func (p *BoxdProvider) machineLabels() map[string]string {
	labels := map[string]string{}
	p.mu.RLock()
	for k, v := range p.labelSet {
		labels[k] = v
	}
	p.mu.RUnlock()
	labels["created-by"] = "ragflow"
	return labels
}

// parseBoxdLabels reads "key=value,key=value".
func parseBoxdLabels(raw string) (map[string]string, error) {
	labels := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !boxdLabelKeyPattern.MatchString(key) || strings.HasPrefix(key, "boxd/") || len(value) > 256 {
			return nil, fmt.Errorf("boxd: invalid label %q: keys are 1-63 lowercase letters, digits and . _ / -, values up to 256 characters", pair)
		}
		labels[key] = value
	}
	return labels, nil
}

// boxdEndpoints derives the gRPC target, whether it uses TLS, and the
// token exchange URL from the base URL and region. fra (or no region)
// uses the base URL's host and nyc uses nyc.<host>; the token comes from
// app.<host>/api/v1/auth/token.
func boxdEndpoints(baseURL, region string) (target string, secure bool, exchangeURL string, err error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" || (u.Path != "" && u.Path != "/") {
		return "", false, "", fmt.Errorf("boxd: invalid API URL %q", baseURL)
	}
	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = boxdAPIPort
	}
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	switch {
	case u.Scheme == "https":
		secure = true
	case u.Scheme == "http" && local:
	default:
		// The API key and the token travel with every call.
		return "", false, "", fmt.Errorf("boxd: API URL must use https: %s", baseURL)
	}
	if region != "" && region != "fra" {
		if !boxdRegionPattern.MatchString(region) {
			return "", false, "", fmt.Errorf("boxd: invalid region %q", region)
		}
		if local || net.ParseIP(host) != nil {
			return "", false, "", fmt.Errorf("boxd: region %q needs an API URL with a host name", region)
		}
	}
	apiHost := host
	if region != "" && region != "fra" {
		apiHost = region + "." + host
	}
	target = net.JoinHostPort(apiHost, port)
	if secure {
		exchangeURL = "https://app." + host + boxdTokenPath
	} else {
		exchangeURL = "http://" + net.JoinHostPort(host, port) + boxdTokenPath
	}
	return target, secure, exchangeURL, nil
}

// boxdTokenCredentials puts the current token on every call.
type boxdTokenCredentials struct {
	provider *BoxdProvider
	secure   bool
}

func (c boxdTokenCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	token, err := c.provider.bearer(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}

func (c boxdTokenCredentials) RequireTransportSecurity() bool { return c.secure }

// bearer returns a token for the API key, exchanging the key when there is
// no token or the token expires within boxdTokenRenew.
func (p *BoxdProvider) bearer(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()
	if p.token != "" && time.Until(p.tokenExpiry) > boxdTokenRenew {
		return p.token, nil
	}
	p.mu.RLock()
	exchangeURL, client := p.exchangeURL, p.httpClient
	p.mu.RUnlock()
	body, err := json.Marshal(map[string]string{"api_key": p.apiKey})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exchangeURL, bytes.NewReader(body))
	if err != nil {
		return "", status.Errorf(codes.Internal, "build token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ragflow-boxd")
	resp, err := client.Do(req)
	if err != nil {
		return "", status.Errorf(codes.Unavailable, "exchange API key: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(raw))
		var answer struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &answer) == nil && (answer.Message != "" || answer.Error != "") {
			message = strings.TrimSpace(answer.Error + " " + answer.Message)
		}
		if len(message) > 300 {
			message = message[:300]
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return "", status.Errorf(codes.Unauthenticated, "the API key was rejected (HTTP %d): %s", resp.StatusCode, message)
		case resp.StatusCode == http.StatusTooManyRequests:
			return "", status.Errorf(codes.ResourceExhausted, "API key exchange is rate limited: %s", message)
		}
		return "", status.Errorf(codes.Unavailable, "exchange API key: HTTP %d: %s", resp.StatusCode, message)
	}
	var answer struct {
		Token     string  `json:"token"`
		ExpiresAt float64 `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil || answer.Token == "" {
		return "", status.Error(codes.Unauthenticated, "exchange API key: answer has no token")
	}
	p.token = answer.Token
	p.tokenExpiry = time.Unix(int64(answer.ExpiresAt), 0)
	return p.token, nil
}

// boxdError names the failed operation and keeps the underlying error,
// so status.Code still reads a gRPC status through it.
type boxdError struct {
	op  string
	err error
}

func (e *boxdError) Error() string {
	if s, ok := status.FromError(e.err); ok && s.Code() != codes.OK {
		return fmt.Sprintf("boxd: %s: %s (%s)", e.op, s.Message(), s.Code())
	}
	return fmt.Sprintf("boxd: %s: %v", e.op, e.err)
}

func (e *boxdError) Unwrap() error { return e.err }

func boxdCallError(op string, err error) error { return &boxdError{op: op, err: err} }

// boxdOutputLimitError ends a command that printed more than its limit.
type boxdOutputLimitError struct{ limit int }

func (e boxdOutputLimitError) Error() string {
	return fmt.Sprintf("boxd: execution output exceeded %d bytes", e.limit)
}

// boxdTransient reports whether a call that failed may succeed if made
// again.
func boxdTransient(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted, codes.Internal, codes.DeadlineExceeded:
		return true
	}
	return false
}

// boxdMaybeCreated reports whether a failed create may still have
// created the machine.
func boxdMaybeCreated(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.Internal, codes.Unknown:
		return true
	}
	return false
}

// boxdWait sleeps for d, or returns ctx's error if ctx ends first.
func boxdWait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// shellQuoteBoxd quotes s for a POSIX shell.
func shellQuoteBoxd(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (p *BoxdProvider) isInitialized() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.initialized
}
