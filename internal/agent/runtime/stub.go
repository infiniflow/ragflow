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

//go:build !ee

package runtime

import "context"

// WithIOProgress is the community-edition stub for enterprise runtime I/O
// progress reporting.
func WithIOProgress(ctx context.Context) (context.Context, func()) {
	return ctx, func() {}
}

// TrackIO is a no-op in the community edition.
func TrackIO(context.Context, string, string, string) func() {
	return func() {}
}

// ReportIOProgress is a no-op in the community edition.
func ReportIOProgress(context.Context, string, string) {}
