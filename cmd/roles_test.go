// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !ee

package main

import "testing"

func TestServerModes(t *testing.T) {
	for _, mode := range []string{"api", "admin", "ingestor", "syncer", "chatter", "migrate"} {
		t.Run(mode, func(t *testing.T) {
			args, err := parseArgsFrom([]string{"server", "--" + mode})
			if err != nil {
				t.Fatal(err)
			}
			if args.Mode == nil || *args.Mode != mode {
				t.Fatalf("mode = %v, want %s", args.Mode, mode)
			}
		})
	}
	args, err := parseArgsFrom([]string{"server"})
	if err != nil || args.Mode != nil {
		t.Fatalf("default mode = %v, err = %v", args.Mode, err)
	}
	if _, err := parseArgsFrom([]string{"server", "--api", "--ingestor"}); err == nil {
		t.Fatal("accepted multiple server modes")
	}
	args, err = parseArgsFrom([]string{"server", "--api=false"})
	if err != nil || args.Mode != nil {
		t.Fatalf("disabled mode selected a service: args=%+v, err=%v", args, err)
	}
}
