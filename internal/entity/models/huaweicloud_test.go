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

package models

import "testing"

func TestHuaweiCloudToolCalls(t *testing.T) {
	newDriver := func(baseURL string) ModelDriver {
		return NewHuaweiCloudModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "v2/chat/completions"})
	}
	t.Run("non-streaming", func(t *testing.T) {
		testNonStreamingToolCall(t, "deepseek-v4-pro", "/v2/chat/completions", newDriver)
	})
	t.Run("streaming", func(t *testing.T) {
		testStreamingToolCall(t, "deepseek-v4-pro", "/v2/chat/completions", newDriver)
	})
}
