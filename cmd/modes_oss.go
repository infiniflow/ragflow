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

import (
	"errors"
	"flag"

	"ragflow/internal/server/bootstrap"
)

func registerFlags(fs *flag.FlagSet, args *bootstrap.ServerArgs) func() error {
	modes := make(map[string]*bool)
	for _, mode := range []string{"api", "admin", "ingestor", "syncer", "chatter", "migrate"} {
		modes[mode] = fs.Bool(mode, false, "")
	}
	return func() error {
		for mode, selected := range modes {
			if !*selected {
				continue
			}
			if args.Mode != nil {
				return errors.New("server modes are mutually exclusive")
			}
			args.Mode = new(mode)
		}
		return nil
	}
}
