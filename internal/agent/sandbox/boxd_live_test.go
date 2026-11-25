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

//go:build integration

// Live test against boxd. It creates real machines and deletes them:
//
//	BOXD_API_KEY=... bash build.sh --test-integration-go -run TestBoxdLive -v ./internal/agent/sandbox/
//
// BOXD_REGION, BOXD_ORG, BOXD_LIVE_NAME_PREFIX and BOXD_LIVE_LABELS
// (key=value,...) are optional.
package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/sandbox/boxdpb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newLiveBoxd(t *testing.T, cfg map[string]any) *BoxdProvider {
	t.Helper()
	full := map[string]any{
		"timeout":     "120",
		"region":      os.Getenv("BOXD_REGION"),
		"org":         os.Getenv("BOXD_ORG"),
		"name_prefix": os.Getenv("BOXD_LIVE_NAME_PREFIX"),
		"labels":      os.Getenv("BOXD_LIVE_LABELS"),
	}
	for k, v := range cfg {
		full[k] = v
	}
	p := newBoxdProviderFromConfig(full)
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return p
}

func TestBoxdLive(t *testing.T) {
	if os.Getenv("BOXD_API_KEY") == "" {
		t.Skip("BOXD_API_KEY is not set")
	}
	ctx := context.Background()
	step := func(name string, start time.Time) { t.Logf("PASS %s (%.2fs)", name, time.Since(start).Seconds()) }
	p := newLiveBoxd(t, nil)

	start := time.Now()
	if err := p.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	step("HealthCheck", start)

	destroy := func(t *testing.T, p *BoxdProvider, inst *SandboxInstance) {
		start := time.Now()
		if err := p.DestroyInstance(ctx, inst); err != nil {
			t.Errorf("DestroyInstance: %v", err)
		}
		_, err := p.api.GetVm(ctx, &boxdpb.GetVmRequest{VmId: inst.InstanceID})
		if status.Code(err) != codes.NotFound {
			t.Errorf("machine %s still answers after DestroyInstance: %v", inst.InstanceID, err)
		}
		step("DestroyInstance "+inst.InstanceID+", then GetVm is NotFound", start)
	}

	t.Run("python", func(t *testing.T) {
		start := time.Now()
		inst, err := p.CreateInstance(ctx, "python")
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		step("CreateInstance python "+inst.InstanceID+" "+inst.Metadata["machine_name"].(string), start)
		defer destroy(t, p, inst)

		vm, err := p.api.GetVm(ctx, &boxdpb.GetVmRequest{VmId: inst.InstanceID})
		if err != nil {
			t.Fatalf("GetVm: %v", err)
		}
		if !vm.GetIsolated() || !slices.Equal(vm.GetEgressAllow(), []string{"none"}) || vm.GetDeleteAtMs() == 0 || vm.GetLabels()["created-by"] != "ragflow" {
			t.Fatalf("machine settings: isolated %v, egress %v, delete_at %d, labels %v",
				vm.GetIsolated(), vm.GetEgressAllow(), vm.GetDeleteAtMs(), vm.GetLabels())
		}
		t.Logf("machine region %q, deletes itself in %s at the latest", vm.GetRegion(),
			time.Until(time.UnixMilli(vm.GetDeleteAtMs())).Round(time.Second))
		step("isolated, egress none, delete-after set", start)

		start = time.Now()
		code := "import os, sys\ndef main(name):\n    os.makedirs('artifacts/sub', exist_ok=True)\n    open('artifacts/out.csv','w').write('a,b\\n1,2\\n')\n    open('artifacts/sub/big.json','w').write('[' + ','.join(['1']*2621440) + ']')\n    print('hello from python')\n    print('to stderr', file=sys.stderr)\n    return {'greeting': 'hi ' + name}\n"
		res, err := p.ExecuteCode(ctx, inst, code, "python", 30, map[string]any{"name": "ragflow"})
		if err != nil {
			t.Fatalf("ExecuteCode: %v", err)
		}
		if res.ExitCode != 0 || !strings.Contains(res.Stdout, "hello from python") || strings.Contains(res.Stdout, "to stderr") || !strings.Contains(res.Stderr, "to stderr") {
			t.Fatalf("ExecuteCode: exit %d stdout %q stderr %q", res.ExitCode, res.Stdout, res.Stderr)
		}
		structured, _ := json.Marshal(res.Metadata["structured_result"])
		if !strings.Contains(string(structured), "hi ragflow") {
			t.Fatalf("structured result: %#v", res.Metadata["structured_result"])
		}
		artifacts, _ := res.Metadata["artifacts"].([]map[string]any)
		if len(artifacts) != 2 || artifacts[0]["name"] != "out.csv" || artifacts[1]["name"] != "sub/big.json" {
			t.Fatalf("artifacts: %d %v", len(artifacts), res.Metadata["artifacts"])
		}
		small, _ := base64.StdEncoding.DecodeString(artifacts[0]["content_b64"].(string))
		big, _ := base64.StdEncoding.DecodeString(artifacts[1]["content_b64"].(string))
		if string(small) != "a,b\n1,2\n" || len(big) != 5242881 {
			t.Fatalf("artifact sizes %d %d", len(small), len(big))
		}
		step("ExecuteCode python: stdout and stderr apart, structured result, 2 artifacts (5 MiB one byte-exact)", start)

		start = time.Now()
		payload := strings.Repeat("x", 5<<20)
		sum := sha256.Sum256([]byte(payload))
		res, err = p.ExecuteCode(ctx, inst, "import hashlib\nDATA = '"+payload+"'\ndef main():\n    return hashlib.sha256(DATA.encode()).hexdigest()\n", "python", 60, nil)
		if err != nil || !strings.Contains(res.Stdout+string(mustJSON(res.Metadata["structured_result"])), hex.EncodeToString(sum[:])) {
			t.Fatalf("5 MiB script: %v %#v", err, res)
		}
		step("5 MiB script uploaded, sha256 matches in the machine", start)

		start = time.Now()
		res, err = p.ExecuteCode(ctx, inst, "import urllib.request\ndef main():\n    try:\n        urllib.request.urlopen('https://example.com', timeout=5)\n        return 'reached'\n    except Exception as e:\n        return 'blocked: ' + type(e).__name__\n", "python", 30, nil)
		if err != nil || !strings.Contains(string(mustJSON(res.Metadata["structured_result"])), "blocked") {
			t.Fatalf("egress: %v %#v", err, res)
		}
		step("outbound network closed: "+string(mustJSON(res.Metadata["structured_result"])), start)

		start = time.Now()
		res, err = p.ExecuteCode(ctx, inst, "import sys\ndef main():\n    sys.exit(3)\n", "python", 30, nil)
		if err != nil || res.ExitCode != 3 {
			t.Fatalf("exit code: %v %#v", err, res)
		}
		step("exit code 3 comes back", start)

		start = time.Now()
		_, err = p.ExecuteCode(ctx, inst, "import time\ndef main():\n    time.sleep(60)\n", "python", 3, nil)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("timeout: %v", err)
		}
		step("timeout 3s returns a timeout error", start)

		start = time.Now()
		_, err = p.ExecuteCode(ctx, inst, "def main():\n    print('y' * (2 << 20))\n", "python", 30, nil)
		if err == nil || !strings.Contains(err.Error(), "output exceeded") {
			t.Fatalf("output limit: %v", err)
		}
		step("2 MiB of output is refused", start)

		start = time.Now()
		res, err = p.ExecuteCode(ctx, inst, "import time\ndef main():\n    time.sleep(65)\n    print('still here')\n", "python", 120, nil)
		if err != nil || !strings.Contains(res.Stdout, "still here") {
			t.Fatalf("silent run: %v %#v", err, res)
		}
		step("a run silent for 65s completes", start)
	})

	t.Run("javascript", func(t *testing.T) {
		start := time.Now()
		inst, err := p.CreateInstance(ctx, "javascript")
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		step("CreateInstance javascript "+inst.InstanceID, start)
		defer destroy(t, p, inst)
		start = time.Now()
		res, err := p.ExecuteCode(ctx, inst, "function main(args) {\n  console.log('hello from node');\n  return {greeting: 'hi ' + args.name};\n}\nmodule.exports = { main };\n", "javascript", 30, map[string]any{"name": "ragflow"})
		if err != nil || res.ExitCode != 0 || !strings.Contains(res.Stdout, "hello from node") || !strings.Contains(string(mustJSON(res.Metadata["structured_result"])), "hi ragflow") {
			t.Fatalf("ExecuteCode: %v %#v", err, res)
		}
		step("ExecuteCode javascript, structured result", start)
	})

	t.Run("internet allowed", func(t *testing.T) {
		q := newLiveBoxd(t, map[string]any{"allow_internet": "true"})
		start := time.Now()
		inst, err := q.CreateInstance(ctx, "python")
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		step("CreateInstance with allow_internet "+inst.InstanceID, start)
		defer destroy(t, q, inst)
		res, err := q.ExecuteCode(ctx, inst, "import urllib.request\ndef main():\n    return urllib.request.urlopen('https://example.com', timeout=10).status\n", "python", 30, nil)
		if err != nil || !strings.Contains(string(mustJSON(res.Metadata["structured_result"])), "200") {
			t.Fatalf("internet: %v %#v", err, res)
		}
		step("allow_internet reaches https://example.com", start)
	})

	t.Run("lost create answer", func(t *testing.T) {
		start := time.Now()
		inst, err := p.CreateInstance(ctx, "python")
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		name := inst.Metadata["machine_name"].(string)
		// The cleanup after a lost create answer deletes by name.
		p.cleanup(ctx, name)
		if _, err := p.api.GetVm(ctx, &boxdpb.GetVmRequest{VmId: inst.InstanceID}); status.Code(err) != codes.NotFound {
			t.Fatalf("machine %s still answers after delete by name: %v", name, err)
		}
		if err := p.DestroyInstance(ctx, inst); err != nil {
			t.Fatalf("DestroyInstance after the machine is gone: %v", err)
		}
		step("delete by name "+inst.InstanceID+"; DestroyInstance on a gone machine succeeds", start)
	})

	t.Run("wrong key", func(t *testing.T) {
		q := newBoxdProviderFromConfig(map[string]any{"api_key": "bxd_" + strings.Repeat("0", 43), "region": os.Getenv("BOXD_REGION")})
		if err := q.Initialize(ctx); err != nil {
			t.Fatal(err)
		}
		err := q.HealthCheck(ctx)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("wrong key: %v", err)
		}
		t.Logf("PASS wrong key: %v", err)
	})
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
