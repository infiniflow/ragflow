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

package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/agent/sandbox/boxdpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeBoxd is an in-memory stand-in for the boxd gRPC API and its token
// exchange endpoint.
type fakeBoxd struct {
	boxdpb.UnimplementedBoxdApiServer
	t *testing.T

	mu          sync.Mutex
	creates     []*boxdpb.CreateVmRequest
	destroyed   []string
	egress      []*boxdpb.SetVmEgressAllowRequest
	commands    []string
	files       map[string][]byte
	auth        []string
	exchanges   int
	nextID      int
	createState string
	getStates   []string
	getErrs     []error
	// createErr is answered after the machine was created.
	createErr   error
	destroyErrs []error
	egressReply []string
	// execErrs fail the next Exec calls before they run.
	execErrs []error
	// listing answers the artifact listing command.
	listing string
	// exec answers a command; nil answers exit 0 with no output.
	exec          func(ctx context.Context, command string, send func(*boxdpb.ExecChunk) error) error
	tokenStatus   int
	tokenLifetime time.Duration
	totalSizeLie  uint64
}

func newFakeBoxd(t *testing.T) *fakeBoxd {
	return &fakeBoxd{t: t, files: map[string][]byte{}, createState: "running", tokenLifetime: time.Hour}
}

func (f *fakeBoxd) recordAuth(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	f.auth = append(f.auth, strings.Join(md.Get("authorization"), ","))
	f.mu.Unlock()
}

func (f *fakeBoxd) CreateVm(ctx context.Context, req *boxdpb.CreateVmRequest) (*boxdpb.CreateVmResponse, error) {
	f.recordAuth(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, req)
	f.nextID++
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &boxdpb.CreateVmResponse{VmId: fmt.Sprintf("vm-%d", f.nextID), Name: req.GetName(), Status: f.createState}, nil
}

func (f *fakeBoxd) GetVm(ctx context.Context, req *boxdpb.GetVmRequest) (*boxdpb.GetVmResponse, error) {
	f.recordAuth(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.getErrs) > 0 {
		err := f.getErrs[0]
		f.getErrs = f.getErrs[1:]
		return nil, err
	}
	state := "running"
	if len(f.getStates) > 0 {
		state, f.getStates = f.getStates[0], f.getStates[1:]
	}
	return &boxdpb.GetVmResponse{VmId: req.GetVmId(), Status: state}, nil
}

func (f *fakeBoxd) DestroyVm(ctx context.Context, req *boxdpb.DestroyVmRequest) (*boxdpb.DestroyVmResponse, error) {
	f.recordAuth(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.destroyErrs) > 0 {
		err := f.destroyErrs[0]
		f.destroyErrs = f.destroyErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	f.destroyed = append(f.destroyed, req.GetVmId())
	return &boxdpb.DestroyVmResponse{}, nil
}

func (f *fakeBoxd) SetVmEgressAllow(ctx context.Context, req *boxdpb.SetVmEgressAllowRequest) (*boxdpb.SetVmEgressAllowResponse, error) {
	f.recordAuth(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.egress = append(f.egress, req)
	entries := req.GetEntries()
	if f.egressReply != nil {
		entries = f.egressReply
	}
	return &boxdpb.SetVmEgressAllowResponse{Entries: entries}, nil
}

func (f *fakeBoxd) Whoami(ctx context.Context, _ *boxdpb.WhoamiRequest) (*boxdpb.WhoamiResponse, error) {
	f.recordAuth(ctx)
	return &boxdpb.WhoamiResponse{UserId: "usr_1"}, nil
}

func (f *fakeBoxd) Exec(stream boxdpb.BoxdApi_ExecServer) error {
	f.recordAuth(stream.Context())
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	// The client sends no stdin: the next read is end of stream.
	if _, err := stream.Recv(); err != io.EOF {
		return status.Errorf(codes.InvalidArgument, "expected end of stdin, got %v", err)
	}
	f.mu.Lock()
	f.commands = append(f.commands, first.GetCommand())
	var failure error
	if len(f.execErrs) > 0 {
		failure, f.execErrs = f.execErrs[0], f.execErrs[1:]
	}
	handler := f.exec
	f.mu.Unlock()
	if failure != nil {
		return failure
	}
	if strings.Contains(first.GetCommand(), "find . -mindepth 1 -printf") {
		f.mu.Lock()
		listing := f.listing
		f.mu.Unlock()
		return execReply(listing, "", 0)(stream.Context(), first.GetCommand(), stream.Send)
	}
	if handler == nil || strings.HasPrefix(first.GetCommand(), "mkdir -p ") {
		return stream.Send(&boxdpb.ExecChunk{ExitCode: 0})
	}
	return handler(stream.Context(), first.GetCommand(), stream.Send)
}

func (f *fakeBoxd) UploadFileStream(stream boxdpb.BoxdApi_UploadFileStreamServer) error {
	var filePath string
	var data []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if chunk.GetPath() != "" {
			filePath = chunk.GetPath()
		}
		data = append(data, chunk.GetData()...)
	}
	f.mu.Lock()
	f.files[filePath] = data
	f.mu.Unlock()
	return stream.SendAndClose(&boxdpb.UploadFileResponse{BytesWritten: uint64(len(data))})
}

func (f *fakeBoxd) DownloadFileStream(req *boxdpb.DownloadFileRequest, stream boxdpb.BoxdApi_DownloadFileStreamServer) error {
	f.mu.Lock()
	data, ok := f.files[req.GetPath()]
	total := uint64(len(data))
	if f.totalSizeLie != 0 {
		total = f.totalSizeLie
	}
	f.mu.Unlock()
	if !ok {
		return status.Error(codes.NotFound, "no such file")
	}
	half := len(data) / 2
	if err := stream.Send(&boxdpb.DownloadFileChunk{TotalSize: total, Data: data[:half]}); err != nil {
		return err
	}
	return stream.Send(&boxdpb.DownloadFileChunk{Data: data[half:]})
}

func (f *fakeBoxd) serveToken(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.exchanges++
	n, code, lifetime := f.exchanges, f.tokenStatus, f.tokenLifetime
	f.mu.Unlock()
	var body struct {
		APIKey string `json:"api_key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if r.URL.Path != boxdTokenPath || body.APIKey != "test-key" {
		code = http.StatusUnauthorized
	}
	if code != 0 && code != http.StatusOK {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"error":"unauthorized","message":"invalid api key"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"token": fmt.Sprintf("tok-%d", n), "expires_at": time.Now().Add(lifetime).Unix()})
}

// newTestBoxd starts the fake and returns an initialized provider that
// talks to it.
func newTestBoxd(t *testing.T, f *fakeBoxd, cfg map[string]any) *BoxdProvider {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	boxdpb.RegisterBoxdApiServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	tokens := httptest.NewServer(http.HandlerFunc(f.serveToken))
	t.Cleanup(tokens.Close)

	full := map[string]any{"api_key": "test-key", "base_url": tokens.URL}
	for k, v := range cfg {
		full[k] = v
	}
	p := newBoxdProviderFromConfig(full)
	p.startPoll = time.Millisecond
	p.deleteBackoff = time.Millisecond
	p.execGrace = 200 * time.Millisecond
	p.dialOptions = []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	})}
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return p
}

func execReply(stdout, stderr string, exit int32) func(context.Context, string, func(*boxdpb.ExecChunk) error) error {
	return func(_ context.Context, _ string, send func(*boxdpb.ExecChunk) error) error {
		if stdout != "" {
			if err := send(&boxdpb.ExecChunk{Data: []byte(stdout)}); err != nil {
				return err
			}
		}
		if stderr != "" {
			if err := send(&boxdpb.ExecChunk{Data: []byte(stderr), IsStderr: true}); err != nil {
				return err
			}
		}
		return send(&boxdpb.ExecChunk{ExitCode: exit})
	}
}

func resultLine(v map[string]any) string {
	raw, _ := json.Marshal(v)
	return resultMarkerPrefix + base64.StdEncoding.EncodeToString(raw) + "\n"
}

func TestBoxdLifecycle(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, map[string]any{"org": "acme", "vcpu": 4, "labels": "team=data,created-by=someone", "max_lifetime": 600})
	f.exec = execReply("hello\n"+resultLine(map[string]any{"present": true, "value": "hi"}), "warn\n", 0)
	ctx := context.Background()

	inst, err := p.CreateInstance(ctx, "python3")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst.InstanceID != "vm-1" || inst.Provider != ProviderBoxd {
		t.Fatalf("instance = %#v", inst)
	}
	req := f.creates[0]
	if !req.GetIsolated() {
		t.Fatal("machine must be isolated")
	}
	if req.GetOrg() != "acme" || !strings.HasPrefix(req.GetName(), "ragflow-sandbox-") || len(req.GetName()) != len("ragflow-sandbox-")+12 {
		t.Fatalf("create request = %v", req)
	}
	if req.GetConfig().GetVcpu() != 4 || req.GetConfig().GetMemoryBytes() != 16<<30 {
		t.Fatalf("size = %d vCPU, %d bytes", req.GetConfig().GetVcpu(), req.GetConfig().GetMemoryBytes())
	}
	if req.GetConfig().GetSrf().AutoSuspendTimeoutSecs == nil || req.GetConfig().GetSrf().GetAutoSuspendTimeoutSecs() != 0 {
		t.Fatal("idle standby must be turned off")
	}
	if req.DeleteAfterSecs == nil || req.GetDeleteAfterSecs() != 600 {
		t.Fatalf("delete_after_secs = %v", req.DeleteAfterSecs)
	}
	if req.GetLabels()["created-by"] != "ragflow" || req.GetLabels()["team"] != "data" {
		t.Fatalf("labels = %v", req.GetLabels())
	}
	if len(f.egress) != 1 || f.egress[0].GetVm() != "vm-1" || strings.Join(f.egress[0].GetEntries(), ",") != "none" {
		t.Fatalf("egress = %v", f.egress)
	}
	workDir := inst.Metadata["remote_work_dir"].(string)
	if !strings.HasPrefix(workDir, "/home/boxd/ragflow-codeexec-") || f.commands[0] != "mkdir -p '"+workDir+"/artifacts'" {
		t.Fatalf("workspace %q, commands %q", workDir, f.commands)
	}

	res, err := p.ExecuteCode(ctx, inst, "def main(name):\n    return 'hi'\n", "python", 0, map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if res.Stdout != "hello\n" || res.Stderr != "warn\n" || res.ExitCode != 0 {
		t.Fatalf("result = %#v", res)
	}
	if res.Metadata["structured_result"].(map[string]any)["value"] != "hi" {
		t.Fatalf("structured = %#v", res.Metadata["structured_result"])
	}
	script := string(f.files[workDir+"/main.py"])
	if !strings.Contains(script, "return 'hi'") {
		t.Fatalf("uploaded script = %q", script)
	}
	if want := "cd '" + workDir + "' && exec timeout -k 5 30 python3 main.py"; f.commands[1] != want {
		t.Fatalf("command = %q, want %q", f.commands[1], want)
	}
	for _, auth := range f.auth {
		if auth != "Bearer tok-1" {
			t.Fatalf("authorization = %q", auth)
		}
	}
	if f.exchanges != 1 {
		t.Fatalf("token exchanges = %d, want 1", f.exchanges)
	}

	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("DestroyInstance: %v", err)
	}
	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("second DestroyInstance: %v", err)
	}
	if strings.Join(f.destroyed, ",") != "vm-1" {
		t.Fatalf("destroyed = %v", f.destroyed)
	}
	if _, err := p.ExecuteCode(ctx, inst, "x", "python", 5, nil); err == nil {
		t.Fatal("ExecuteCode on a destroyed instance succeeded")
	}
}

func TestBoxdNodeAndInternet(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, map[string]any{"allow_internet": true, "name_prefix": "t-rag-"})
	f.exec = execReply("", "", 0)
	inst, err := p.CreateInstance(context.Background(), "javascript")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.egress) != 0 {
		t.Fatalf("egress changed with allow_internet: %v", f.egress)
	}
	if !strings.HasPrefix(f.creates[0].GetName(), "t-rag-") || f.creates[0].GetConfig().GetVcpu() != 0 || f.creates[0].GetConfig().GetMemoryBytes() != 0 {
		t.Fatalf("create = %v", f.creates[0])
	}
	if _, err := p.ExecuteCode(context.Background(), inst, "function main(){}", "js", 7, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(f.commands[1], "exec timeout -k 5 7 node main.js") {
		t.Fatalf("command = %q", f.commands[1])
	}
}

func TestBoxdWaitsUntilRunningAndRetriesFirstCommand(t *testing.T) {
	f := newFakeBoxd(t)
	f.createState = "starting"
	f.getStates = []string{"pending", "starting", "running"}
	f.execErrs = []error{status.Error(codes.Unavailable, "not ready"), status.Error(codes.Unavailable, "not ready")}
	p := newTestBoxd(t, f, nil)
	if _, err := p.CreateInstance(context.Background(), "python"); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if len(f.getStates) != 0 || len(f.commands) != 3 {
		t.Fatalf("states left %v, commands %q", f.getStates, f.commands)
	}
}

func TestBoxdStartPollsThroughNotFound(t *testing.T) {
	f := newFakeBoxd(t)
	f.createState = "starting"
	f.getErrs = []error{status.Error(codes.NotFound, "vm not found"), status.Error(codes.NotFound, "vm not found")}
	p := newTestBoxd(t, f, nil)
	if _, err := p.CreateInstance(context.Background(), "python"); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if len(f.getErrs) != 0 || len(f.destroyed) != 0 {
		t.Fatalf("errors left %d, destroyed %v", len(f.getErrs), f.destroyed)
	}

	f = newFakeBoxd(t)
	f.createState = "starting"
	f.getErrs = []error{status.Error(codes.PermissionDenied, "no")}
	p = newTestBoxd(t, f, nil)
	if _, err := p.CreateInstance(context.Background(), "python"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("err = %v", err)
	}
}

func TestBoxdFailedStartDeletesMachine(t *testing.T) {
	cases := map[string]func(*fakeBoxd){
		"failed state":   func(f *fakeBoxd) { f.createState = "starting"; f.getStates = []string{"failed"} },
		"egress refused": func(f *fakeBoxd) { f.egressReply = []string{} },
		"mkdir fails": func(f *fakeBoxd) {
			f.execErrs = []error{status.Error(codes.PermissionDenied, "no")}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeBoxd(t)
			setup(f)
			p := newTestBoxd(t, f, nil)
			if _, err := p.CreateInstance(context.Background(), "python"); err == nil {
				t.Fatal("CreateInstance succeeded")
			}
			if strings.Join(f.destroyed, ",") != "vm-1" {
				t.Fatalf("destroyed = %v, want vm-1", f.destroyed)
			}
		})
	}
}

func TestBoxdStartTimeoutDeletesMachine(t *testing.T) {
	f := newFakeBoxd(t)
	f.createState = "starting"
	f.getStates = make([]string, 10000)
	for i := range f.getStates {
		f.getStates[i] = "starting"
	}
	p := newTestBoxd(t, f, nil)
	p.startTimeout = 50 * time.Millisecond
	_, err := p.CreateInstance(context.Background(), "python")
	if err == nil || !strings.Contains(err.Error(), "still starting") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(f.destroyed, ",") != "vm-1" {
		t.Fatalf("destroyed = %v", f.destroyed)
	}
}

func TestBoxdLostCreateAnswerDeletesByName(t *testing.T) {
	f := newFakeBoxd(t)
	f.createErr = status.Error(codes.Unavailable, "connection reset")
	p := newTestBoxd(t, f, nil)
	if _, err := p.CreateInstance(context.Background(), "python"); err == nil {
		t.Fatal("CreateInstance succeeded")
	}
	if len(f.destroyed) != 1 || f.destroyed[0] != f.creates[0].GetName() {
		t.Fatalf("destroyed = %v, want the machine's name %q", f.destroyed, f.creates[0].GetName())
	}

	f = newFakeBoxd(t)
	f.createErr = status.Error(codes.InvalidArgument, "bad size")
	p = newTestBoxd(t, f, nil)
	_, err := p.CreateInstance(context.Background(), "python")
	if err == nil || status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), "boxd: create machine: bad size") {
		t.Fatalf("err = %v", err)
	}
	if len(f.destroyed) != 0 {
		t.Fatalf("a refused create was deleted: %v", f.destroyed)
	}
}

func TestBoxdTimeouts(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, map[string]any{"timeout": 1})
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatal(err)
	}

	// `timeout` stopped the command.
	f.exec = func(ctx context.Context, command string, send func(*boxdpb.ExecChunk) error) error {
		time.Sleep(1100 * time.Millisecond)
		return send(&boxdpb.ExecChunk{ExitCode: 124})
	}
	if _, err := p.ExecuteCode(context.Background(), inst, "x", "python", 30, nil); err == nil || !strings.Contains(err.Error(), "timed out after 1 seconds") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(f.commands[len(f.commands)-1], "timeout -k 5 1 python3") {
		t.Fatalf("the admin timeout must cap the run: %q", f.commands[len(f.commands)-1])
	}

	// The code exits 124 itself, quickly: not a timeout.
	f.exec = execReply("", "", 124)
	res, err := p.ExecuteCode(context.Background(), inst, "x", "python", 1, nil)
	if err != nil || res.ExitCode != 124 {
		t.Fatalf("res %#v err %v", res, err)
	}

	// The machine never answers: the call is cancelled after the grace.
	cancelled := make(chan struct{})
	f.exec = func(ctx context.Context, _ string, _ func(*boxdpb.ExecChunk) error) error {
		<-ctx.Done()
		close(cancelled)
		return ctx.Err()
	}
	start := time.Now()
	if _, err := p.ExecuteCode(context.Background(), inst, "x", "python", 1, nil); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the remote command was not cancelled")
	}

	if _, err := p.ExecuteCode(context.Background(), inst, "x", "python", 601, nil); err == nil {
		t.Fatal("timeout above 600 accepted")
	}
}

func TestBoxdStreamWithoutExitStatus(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, nil)
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatal(err)
	}
	// Output, then the stream closes without the final exit chunk.
	f.exec = func(_ context.Context, _ string, send func(*boxdpb.ExecChunk) error) error {
		return send(&boxdpb.ExecChunk{Data: []byte("partial")})
	}
	_, err = p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
	if err == nil || !strings.Contains(err.Error(), "ended without an exit status") {
		t.Fatalf("err = %v", err)
	}
	// Output followed by its exit chunk is a normal end.
	f.exec = execReply("done\n", "", 7)
	res, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
	if err != nil || res.ExitCode != 7 || res.Stdout != "done\n" {
		t.Fatalf("res %#v err %v", res, err)
	}
}

func TestBoxdOutputLimit(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, map[string]any{"max_output_bytes": 1024})
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatal(err)
	}
	f.exec = func(ctx context.Context, _ string, send func(*boxdpb.ExecChunk) error) error {
		for {
			if err := send(&boxdpb.ExecChunk{Data: []byte(strings.Repeat("x", 600))}); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(time.Millisecond)
		}
	}
	_, err = p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
	if err == nil || err.Error() != "boxd: execution output exceeded 1024 bytes" {
		t.Fatalf("err = %v", err)
	}
}

func TestBoxdArtifacts(t *testing.T) {
	setup := func(t *testing.T, cfg map[string]any, listing string, files map[string]string) (*BoxdProvider, *fakeBoxd, *SandboxInstance) {
		f := newFakeBoxd(t)
		p := newTestBoxd(t, f, cfg)
		inst, err := p.CreateInstance(context.Background(), "python")
		if err != nil {
			t.Fatal(err)
		}
		dir := inst.Metadata["remote_work_dir"].(string) + "/artifacts/"
		for name, content := range files {
			f.files[dir+name] = []byte(content)
		}
		f.listing = listing
		f.exec = execReply("ok\n", "", 0)
		t.Cleanup(func() {
			last := f.commands[len(f.commands)-1]
			if !strings.HasPrefix(last, "cd '"+strings.TrimSuffix(dir, "/")+"' 2>/dev/null || exit 0;") {
				t.Errorf("listing command = %q", last)
			}
		})
		return p, f, inst
	}

	t.Run("collected depth-first by name", func(t *testing.T) {
		listing := "f\t3\tz.json\x00d\t4096\tsub\x00f\t8\tout.csv\x00f\t2\tsub/a.png\x00"
		p, _, inst := setup(t, nil, listing, map[string]string{"z.json": "{}\n", "out.csv": "a,b\n1,2\n", "sub/a.png": "PN"})
		res, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
		if err != nil {
			t.Fatal(err)
		}
		artifacts := res.Metadata["artifacts"].([]map[string]any)
		var names []string
		for _, a := range artifacts {
			names = append(names, a["name"].(string))
		}
		if strings.Join(names, ",") != "out.csv,sub/a.png,z.json" {
			t.Fatalf("names = %v", names)
		}
		content, _ := base64.StdEncoding.DecodeString(artifacts[0]["content_b64"].(string))
		if string(content) != "a,b\n1,2\n" || artifacts[0]["mime_type"] != "text/csv; charset=utf-8" || artifacts[0]["size"] != int64(8) {
			t.Fatalf("artifact = %#v", artifacts[0])
		}
	})

	t.Run("no artifacts", func(t *testing.T) {
		p, _, inst := setup(t, nil, "", nil)
		res, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Metadata["artifacts"].([]map[string]any); len(got) != 0 {
			t.Fatalf("artifacts = %v", got)
		}
	})

	rejected := []struct {
		name, listing, want string
		cfg                 map[string]any
		files               map[string]string
	}{
		{"symlink", "l\t9\tlink.csv\x00", "symlinks and special files are not allowed: link.csv", nil, nil},
		{"extension", "f\t3\trun.sh\x00", "unsupported artifact type: run.sh", nil, nil},
		{"count", "f\t1\ta.csv\x00f\t1\tb.csv\x00", "more than 1 artifacts", map[string]any{"max_artifacts": 1}, map[string]string{"a.csv": "1", "b.csv": "2"}},
		{"listed size", "f\t2048\tbig.csv\x00", "artifact exceeds 1024 bytes: big.csv", map[string]any{"max_artifact_bytes": 1024}, nil},
		{"grown file", "f\t1\tbig.csv\x00", "over the 1024 byte limit", map[string]any{"max_artifact_bytes": 1024}, map[string]string{"big.csv": strings.Repeat("x", 2048)}},
		{"depth", "f\t1\t" + strings.Repeat("d/", boxdMaxArtifactDepth+1) + "a.csv\x00", "nesting exceeds", nil, nil},
		{"escape", "f\t1\t../a.csv\x00", "invalid artifact path", nil, nil},
		{"garbage", "nonsense\x00", "bad artifact listing entry", nil, nil},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			p, _, inst := setup(t, tc.cfg, tc.listing, tc.files)
			_, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}

	t.Run("truncated download", func(t *testing.T) {
		p, f, inst := setup(t, nil, "f\t4\ta.csv\x00", map[string]string{"a.csv": "1234"})
		f.totalSizeLie = 8
		_, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
		if err == nil || !strings.Contains(err.Error(), "download truncated: got 4 of 8 bytes") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestBoxdDestroyRetriesAndKeepsRecord(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, nil)
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := status.Error(codes.Unavailable, "try again")
	f.destroyErrs = []error{unavailable, unavailable, unavailable}
	if err := p.DestroyInstance(context.Background(), inst); err == nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v", err)
	}
	if len(f.destroyed) != 0 || len(f.destroyErrs) != 0 {
		t.Fatalf("destroyed %v, errors left %d", f.destroyed, len(f.destroyErrs))
	}
	// The record is kept, so the next DestroyInstance deletes it.
	f.destroyErrs = []error{unavailable}
	if err := p.DestroyInstance(context.Background(), inst); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if strings.Join(f.destroyed, ",") != "vm-1" {
		t.Fatalf("destroyed = %v", f.destroyed)
	}

	// Gone already counts as deleted; a refusal is not retried.
	inst2, _ := p.CreateInstance(context.Background(), "python")
	f.destroyErrs = []error{status.Error(codes.NotFound, "vm not found")}
	if err := p.DestroyInstance(context.Background(), inst2); err != nil {
		t.Fatalf("not found: %v", err)
	}
	inst3, _ := p.CreateInstance(context.Background(), "python")
	f.destroyErrs = []error{status.Error(codes.PermissionDenied, "no"), nil}
	if err := p.DestroyInstance(context.Background(), inst3); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("err = %v", err)
	}
	if len(f.destroyErrs) != 1 {
		t.Fatal("a refused delete was retried")
	}
}

func TestBoxdTokenRenewal(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, nil)
	for range 3 {
		if err := p.HealthCheck(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if f.exchanges != 1 {
		t.Fatalf("exchanges = %d, want 1", f.exchanges)
	}
	// A token that expires within the renewal window is replaced.
	f.tokenLifetime = time.Minute
	p.tokenMu.Lock()
	p.tokenExpiry = time.Now().Add(time.Minute)
	p.tokenMu.Unlock()
	for range 2 {
		if err := p.HealthCheck(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if f.exchanges != 3 || f.auth[len(f.auth)-1] != "Bearer tok-3" {
		t.Fatalf("exchanges = %d, last auth %q", f.exchanges, f.auth[len(f.auth)-1])
	}
}

func TestBoxdRejectedKey(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, map[string]any{"api_key": "wrong-key"})
	err := p.HealthCheck(context.Background())
	if err == nil || status.Code(err) != codes.Unauthenticated || !strings.Contains(err.Error(), "the API key was rejected (HTTP 401)") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "wrong-key") {
		t.Fatal("the error carries the API key")
	}
	_, err = p.CreateInstance(context.Background(), "python")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateInstance err = %v", err)
	}
	if len(f.creates) != 0 || len(f.destroyed) != 0 {
		t.Fatalf("calls reached the API: creates %d, destroys %d", len(f.creates), len(f.destroyed))
	}
}

func TestBoxdInitializeValidation(t *testing.T) {
	t.Setenv("BOXD_API_KEY", "")
	t.Setenv("BOXD_BASE_URL", "")
	cases := []struct {
		cfg  map[string]any
		want string
	}{
		{map[string]any{}, "API key is required"},
		{map[string]any{"api_key": "k", "base_url": "http://boxd.example.com"}, "must use https"},
		{map[string]any{"api_key": "k", "base_url": "https://boxd.sh/api"}, "invalid API URL"},
		{map[string]any{"api_key": "k", "vcpu": 3}, "vcpu must be"},
		{map[string]any{"api_key": "k", "name_prefix": "Bad_"}, "name_prefix"},
		{map[string]any{"api_key": "k", "labels": "boxd/x=y"}, "invalid label"},
		{map[string]any{"api_key": "k", "region": "New York"}, "invalid region"},
		{map[string]any{"api_key": "k", "region": "nyc", "base_url": "https://10.0.0.1:9443"}, "needs an API URL with a host name"},
		{map[string]any{"api_key": "k", "max_lifetime": 30}, "max_lifetime at least 60"},
	}
	for _, tc := range cases {
		err := newBoxdProviderFromConfig(tc.cfg).Initialize(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.cfg, err, tc.want)
		}
	}
	t.Setenv("BOXD_API_KEY", "env-key")
	p := newBoxdProviderFromEnv()
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("env key: %v", err)
	}
	if p.apiKey != "env-key" || p.baseURL != boxdDefaultBaseURL {
		t.Fatalf("apiKey/baseURL from env = %q %q", p.apiKey, p.baseURL)
	}
	if _, err := newBoxdProviderFromConfig(nil).CreateInstance(context.Background(), "python"); err == nil {
		t.Fatal("CreateInstance before Initialize succeeded")
	}
}

func TestBoxdEndpoints(t *testing.T) {
	cases := []struct {
		base, region, target, exchange string
		secure                         bool
	}{
		{"https://boxd.sh:9443", "", "boxd.sh:9443", "https://app.boxd.sh/api/v1/auth/token", true},
		{"https://boxd.sh", "fra", "boxd.sh:9443", "https://app.boxd.sh/api/v1/auth/token", true},
		{"https://boxd.sh:9443", "nyc", "nyc.boxd.sh:9443", "https://app.boxd.sh/api/v1/auth/token", true},
		{"http://127.0.0.1:8080", "", "127.0.0.1:8080", "http://127.0.0.1:8080/api/v1/auth/token", false},
	}
	for _, tc := range cases {
		target, secure, exchange, err := boxdEndpoints(tc.base, tc.region)
		if err != nil || target != tc.target || secure != tc.secure || exchange != tc.exchange {
			t.Errorf("%s %s: %q %v %q %v", tc.base, tc.region, target, secure, exchange, err)
		}
	}
}

func TestBoxdUnsupportedLanguage(t *testing.T) {
	f := newFakeBoxd(t)
	p := newTestBoxd(t, f, nil)
	if _, err := p.CreateInstance(context.Background(), "ruby"); err == nil {
		t.Fatal("ruby accepted")
	}
	if len(f.creates) != 0 {
		t.Fatal("a machine was created for an unsupported language")
	}
	var callErr *boxdError
	err := boxdCallError("x", status.Error(codes.NotFound, "gone"))
	if !errors.As(err, &callErr) || status.Code(err) != codes.NotFound || err.Error() != "boxd: x: gone (NotFound)" {
		t.Fatalf("err = %v", err)
	}
}
