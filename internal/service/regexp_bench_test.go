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

package service

import "testing"

// BenchmarkCleanTTSText guards the regexp-hoist optimization on a cleanup
// helper that compiles four fixed regexps per call today.
func BenchmarkCleanTTSText(b *testing.B) {
	b.ReportAllocs()
	in := "hello\x01world 😀 <b>bold</b>\n\n  text"
	for i := 0; i < b.N; i++ {
		cleanTTSText(in)
	}
}
