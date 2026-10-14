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
// package both the canvas agent and the smart-reasoning agent depend on). This
// file re-exports them under the historical tool.XXX names so the canvas tool
// package keeps its public API stable without owning a second copy.
package tool

import (
	"ragflow/internal/agent/runtime"
)

// Re-exported retrieval contracts (single owner: internal/agent/runtime).
type (
	RetrievalChunk         = runtime.RetrievalChunk
	RetrievalRequest       = runtime.RetrievalRequest
	GrepRequest            = runtime.GrepRequest
	RetrievalService       = runtime.RetrievalService
	MemoryRetrievalService = runtime.MemoryRetrievalService
	KGRetrievalService     = runtime.KGRetrievalService
	GrepService            = runtime.GrepService
)

// ErrRetrievalServiceMissing is declared in retrieval.go so callers and the
// default stub share the same sentinel.
var (
	ErrRetrievalServiceMissing       = runtime.ErrRetrievalServiceMissing
	ErrMemoryRetrievalServiceMissing = runtime.ErrMemoryRetrievalServiceMissing
	ErrKGRetrievalServiceMissing     = runtime.ErrKGRetrievalServiceMissing
	ErrGrepServiceMissing            = runtime.ErrGrepServiceMissing
	ErrRegexpNotSupported            = runtime.ErrRegexpNotSupported
)

func SetRetrievalService(svc RetrievalService) { runtime.SetRetrievalService(svc) }
func GetRetrievalService() RetrievalService    { return runtime.GetRetrievalService() }

func SetMemoryRetrievalService(svc MemoryRetrievalService) { runtime.SetMemoryRetrievalService(svc) }
func GetMemoryRetrievalService() MemoryRetrievalService    { return runtime.GetMemoryRetrievalService() }

func SetKGRetrievalService(svc KGRetrievalService) { runtime.SetKGRetrievalService(svc) }
func GetKGRetrievalService() KGRetrievalService    { return runtime.GetKGRetrievalService() }

func SetGrepService(svc GrepService) { runtime.SetGrepService(svc) }
func GetGrepService() GrepService    { return runtime.GetGrepService() }

// Bm25Service mirrors runtime.Bm25Service for injection outside this package.
type Bm25Service = runtime.Bm25Service

func SetBm25Service(svc Bm25Service) { runtime.SetBm25Service(svc) }
func GetBm25Service() Bm25Service    { return runtime.GetBm25Service() }

// SetSimpleRetrievalService installs deterministic synthetic retrieval for
// tests and local demos.
func SetSimpleRetrievalService() { runtime.SetSimpleRetrievalService() }
