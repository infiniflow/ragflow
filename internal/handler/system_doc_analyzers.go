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

package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

// DocAnalyzerInfo describes one PDF doc-analyzer (table/structure engine)
// the deployment can serve. Availability is probed per backend so the
// front end renders only engines that are actually usable.
//
// NOTE: a probed backend endpoint URL is intentionally NOT exposed here. The
// availability flag is enough for the UI to decide what to render; leaking
// an internal service address to clients adds no value and broadens the
// information surface.
type DocAnalyzerInfo struct {
	// ID is the parse_method the engine maps to (e.g. "deepdoc").
	ID string `json:"id"`
	// Kind is "builtin" for the in-process (Go) DeepDoc backend; additional
	// backends may be contributed by a provider hook.
	Kind string `json:"kind"`
	// Available reports whether the backend can currently serve: the
	// builtin entry checks local model weights.
	Available bool `json:"available"`
	// Label is a human-readable name for the UI.
	Label string `json:"label,omitempty"`
}

// docAnalyzers returns the merged, availability-probed list of doc
// analyzers. The in-process DeepDoc backend is always present (builtin);
// optional HTTP backends are contributed by a provider hook. The probe runs
// on every call: this is a low-frequency health check and the cost (a
// filesystem stat) does not warrant memoization, so there is no
// package-global cache to go stale.
func docAnalyzers(ctx context.Context) []DocAnalyzerInfo {
	if ctx == nil {
		ctx = context.Background()
	}
	list := []DocAnalyzerInfo{
		{
			ID:        "deepdoc",
			Kind:      "builtin",
			Available: common.HasModelFiles(common.DeepDocModelDir()),
			Label:     "DeepDoc (in-process)",
		},
	}
	list = append(list, eeDocAnalyzers(ctx)...)
	return list
}

// DocAnalyzers handles GET /api/v1/system/doc-analyzers, returning the
// merged availability-probed list of doc analyzers so the front end can
// offer only engines this deployment can actually run.
func (h *SystemHandler) DocAnalyzers(c *gin.Context) {
	common.RespondSuccess(c, docAnalyzers(c.Request.Context()), "success")
}
