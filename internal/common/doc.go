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

// Package common holds the cross-cutting primitives shared across the whole
// tree: process-wide concerns (logging, environment access, version, error
// codes), the HTTP response envelope, and small stateless helpers that have no
// single owning domain. It is a leaf package — it imports nothing from
// internal/* — so any package may depend on it without forming a cycle.
//
// That reach is exactly why admission must be deliberate: anything placed here
// becomes visible to every layer and is hard to remove later. The explicit
// allowlist in placement_test.go is the machine-enforced contract. Adding a new
// file requires BOTH registering it in that allowlist AND deciding that it
// satisfies the rules below.
//
// Placement rules, in priority order (stop at the first match):
//
//  1. A single consumer owns the concern → put it in that package, not here.
//     (Example: query-rewrite prompt/parse lives in internal/service/graph.)
//  2. An application config value or config parser → put it in
//     internal/server/config with the type it populates.
//     (Example: SMTPConfig moved here from common.)
//  3. A domain concept (tasks, models, metadata, components, parsing,
//     multi-modal content) → put it in the owning domain package.
//  4. A stateless algorithm over data (type/string/math conversion, file and
//     network utilities, SSRF-guarded HTTP clients) → put it in
//     internal/utility.
//  5. A cross-layer contract with no owner (response envelope, error codes,
//     logging entry points, environment access) → this package.
//
// A file is never admitted merely because "many packages might use it". Reach
// alone is not ownership. If a symbol would have exactly one caller today,
// rule 1 wins regardless of hypothetical future reach.
package common
