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
	"reflect"
	"testing"
)

func TestCommonArguments(t *testing.T) {
	want, err := parseArgsFrom([]string{"server", "--config", "cfg=local.yaml", "--log-level", "debug", "--profile"})
	if err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{
		{"server", "-f=cfg=local.yaml", "--log-level=debug", "--profile=true"},
		{"server", "-config", "cfg=local.yaml", "-log-level", "debug", "-profile"},
	} {
		got, err := parseArgsFrom(argv)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%v: got %+v, err %v; want %+v", argv, got, err, want)
		}
	}
	for _, flag := range []string{"--help", "-h", "--version", "-v"} {
		args, err := parseArgsFrom([]string{"server", flag})
		if err != nil || !(args.HelpFlag || args.VersionFlag) {
			t.Fatalf("%s: args=%+v, err=%v", flag, args, err)
		}
	}
	for _, argv := range [][]string{{"--config"}, {"--log-level=trace"}, {"--profile=maybe"}, {"unexpected"}} {
		if _, err := parseArgsFrom(append([]string{"server"}, argv...)); err == nil {
			t.Fatalf("accepted invalid arguments %v", argv)
		}
	}
}
