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

package pipeline

import (
	"errors"
	"strconv"
)

// Sentinel errors returned by the pipeline package. Callers
// should compare with errors.Is so the messages can evolve.
var (
	errNilDSL             = errors.New("pipeline: nil DSL")
	errEmptyStages        = errors.New("pipeline: DSL has no stages")
	errStageCountMismatch = errors.New("pipeline: stage_count does not match len(stages)")
	errUnknownComponent   = errors.New("pipeline: unknown component")
	errUnknownStage       = errors.New("pipeline: unknown stage")
)

func errEmptyStageType(idx int) error {
	return &stageError{Stage: strconv.Itoa(idx), Reason: "empty type"}
}

func errDuplicateStage(typeName string) error {
	return &stageError{Stage: typeName, Reason: "duplicate stage type"}
}

// stageError is a small structured error type for stage-level
// validation. It carries the offending stage's type name (or
// index as a string for the empty-type case) plus a reason.
// Callers can errors.As to it for programmatic handling.
type stageError struct {
	Stage  string
	Reason string
}

func (e *stageError) Error() string {
	return "pipeline: stage " + e.Stage + ": " + e.Reason
}

// sinkError is a small structured error type for ProgressSink
// misconfigurations (nil DAO, missing factory, etc.).
type sinkError struct {
	Reason string
}

func (e *sinkError) Error() string {
	return "pipeline: sink: " + e.Reason
}
