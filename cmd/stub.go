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

	deepdocserver "ragflow/internal/deepdoc/server"
	"ragflow/internal/server/bootstrap"
)

// runDeepDoc is the stub entrypoint of the --deepdoc mode: it only hands the
// call to the edition-selected implementation in internal/deepdoc/server, which
// serves inference when the ee tag is set and reports the capability as
// enterprise-only otherwise. Nothing edition-aware lives here, so build.sh links
// this file for both editions.
func runDeepDoc(ctx context.Context, serverName string, args *bootstrap.ServerArgs) error {
	return deepdocserver.RunServer(ctx, serverName, args.Port)
}
