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

// Retrieval contracts live in internal/agent/runtime (the engine-agnostic
// package the canvas agent depends on). This file re-exports them under the
// historical tool.XXX names so the canvas tool package keeps its public API
// stable without owning a second copy.
package tool

import (
	"ragflow/internal/agent/runtime"
)

// Re-exported retrieval contracts (single owner: internal/agent/runtime).
type (
	RetrievalChunk         = runtime.RetrievalChunk
	RetrievalRequest       = runtime.RetrievalRequest
	RetrievalService       = runtime.RetrievalService
	MemoryRetrievalService = runtime.MemoryRetrievalService
	KGRetrievalService     = runtime.KGRetrievalService
)

// ErrRetrievalServiceMissing is declared in retrieval.go so callers and the
// default stub share the same sentinel.
var (
	ErrRetrievalServiceMissing       = runtime.ErrRetrievalServiceMissing
	ErrMemoryRetrievalServiceMissing = runtime.ErrMemoryRetrievalServiceMissing
	ErrKGRetrievalServiceMissing     = runtime.ErrKGRetrievalServiceMissing
)

func SetRetrievalService(svc RetrievalService) { runtime.SetRetrievalService(svc) }
func GetRetrievalService() RetrievalService    { return runtime.GetRetrievalService() }

func SetMemoryRetrievalService(svc MemoryRetrievalService) { runtime.SetMemoryRetrievalService(svc) }
func GetMemoryRetrievalService() MemoryRetrievalService    { return runtime.GetMemoryRetrievalService() }

func SetKGRetrievalService(svc KGRetrievalService) { runtime.SetKGRetrievalService(svc) }
func GetKGRetrievalService() KGRetrievalService    { return runtime.GetKGRetrievalService() }

// SetSimpleRetrievalService installs deterministic synthetic retrieval for
// tests and local demos.
func SetSimpleRetrievalService() { runtime.SetSimpleRetrievalService() }
