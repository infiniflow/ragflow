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

package compliance

import "context"

func ValidateQuestion(ctx context.Context, question string) error {
	return nil
}

func WithOutputFilter(ctx context.Context) (context.Context, error) {
	return ctx, ctx.Err()
}

func FilterOutput(ctx context.Context, text string) (string, error) {
	return text, ctx.Err()
}

func FilterText(_ context.Context, text string) string { return text }

func OutputFilterEnabled(context.Context) bool { return false }

func ValidateSetting(name, value string) error { return nil }
