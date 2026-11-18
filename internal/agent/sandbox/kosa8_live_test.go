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

//go:build kosa8live

// Live test against kosa8 Cloud. It creates real sandboxes and deletes them:
//
//	KOSA8_API_KEY=... go test -tags kosa8live -run TestKosa8Live -v ./internal/agent/sandbox
package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestKosa8Live(t *testing.T) {
	if os.Getenv("KOSA8_API_KEY") == "" {
		t.Skip("KOSA8_API_KEY is not set")
	}
	ctx := context.Background()
	p := newKosa8ProviderFromConfig(map[string]any{"timeout": "120"})
	if err := p.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	step := func(name string, start time.Time) { t.Logf("PASS %s (%.2fs)", name, time.Since(start).Seconds()) }

	start := time.Now()
	if err := p.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	step("HealthCheck", start)

	cases := []struct {
		language, code, want string
		artifact             bool
	}{
		{"python", "import os\ndef main(name):\n    os.makedirs('artifacts', exist_ok=True)\n    open('artifacts/out.csv','w').write('a,b\\n1,2\\n')\n    print('hello from python')\n    return {'greeting': 'hi ' + name}\n", "hello from python", true},
		{"javascript", "function main(args) {\n  console.log('hello from node');\n  return {greeting: 'hi ' + args.name};\n}\nmodule.exports = { main };\n", "hello from node", false},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			start := time.Now()
			inst, err := p.CreateInstance(ctx, tc.language)
			if err != nil {
				t.Fatalf("CreateInstance: %v", err)
			}
			step("CreateInstance "+tc.language+" "+inst.InstanceID, start)
			defer func() {
				start := time.Now()
				if err := p.DestroyInstance(ctx, inst); err != nil {
					t.Errorf("DestroyInstance: %v", err)
				}
				var gone *kosa8APIError
				err := p.call(ctx, "GET", p.sandboxPath(inst.InstanceID), nil, nil)
				if err == nil || !errors.As(err, &gone) || gone.Status != 404 {
					t.Errorf("sandbox %s still answers after DestroyInstance: %v", inst.InstanceID, err)
				}
				step("DestroyInstance "+tc.language, start)
			}()

			start = time.Now()
			res, err := p.ExecuteCode(ctx, inst, tc.code, tc.language, 30, map[string]any{"name": "ragflow"})
			if err != nil {
				t.Fatalf("ExecuteCode: %v", err)
			}
			if res.ExitCode != 0 || !strings.Contains(res.Stdout, tc.want) {
				t.Fatalf("ExecuteCode: exit %d stdout %q stderr %q", res.ExitCode, res.Stdout, res.Stderr)
			}
			structured, _ := json.Marshal(res.Metadata["structured_result"])
			if !strings.Contains(string(structured), "hi ragflow") {
				t.Fatalf("structured result: %#v", res.Metadata["structured_result"])
			}
			step("ExecuteCode "+tc.language, start)

			if tc.artifact {
				artifacts, _ := res.Metadata["artifacts"].([]map[string]any)
				if len(artifacts) != 1 || artifacts[0]["name"] != "out.csv" {
					t.Fatalf("artifacts: %#v", res.Metadata["artifacts"])
				}
				content, _ := base64.StdEncoding.DecodeString(artifacts[0]["content_b64"].(string))
				if string(content) != "a,b\n1,2\n" {
					t.Fatalf("artifact content %q", content)
				}
				step("artifacts", start)

				start = time.Now()
				_, err := p.ExecuteCode(ctx, inst, "import time\ndef main():\n    time.sleep(60)\n", "python", 3, nil)
				if err == nil || !strings.Contains(err.Error(), "timed out") {
					t.Fatalf("buffered timeout: %v", err)
				}
				step("timeout 3s (buffered)", start)

				start = time.Now()
				res, err = p.ExecuteCode(ctx, inst, "def main():\n    print('long')\n", "python", 120, nil)
				if err != nil || !strings.Contains(res.Stdout, "long") {
					t.Fatalf("streamed exec: %v %#v", err, res)
				}
				step("ExecuteCode timeout 120s (streamed)", start)

				start = time.Now()
				_, err = p.ExecuteCode(ctx, inst, "import time\ndef main():\n    time.sleep(300)\n", "python", 92, nil)
				if err == nil || !strings.Contains(err.Error(), "timed out") {
					t.Fatalf("streamed timeout: %v", err)
				}
				step("timeout 92s (streamed)", start)
			}
		})
	}
}
