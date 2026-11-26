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

package source

import "testing"

// BenchmarkExtractQueryTerms guards the regexp-hoist optimization: this helper
// is called O(N + 2N*logN) times during result ranking, so per-call regexp
// compilation is a hot path.
func BenchmarkExtractQueryTerms(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		extractQueryTerms("Hello_World-And.Some Terms")
	}
}
