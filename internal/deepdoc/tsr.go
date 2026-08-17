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

package deepdoc

import "context"

// TSR is a stub. The Python deepdoc service has no remote TSR
// endpoint — table structure recognition is a 100% local ONNX
// pipeline (deepdoc/vision/table_structure_recognizer.py:30).
// Callers that need TSR must keep using the Python deepdoc
// service directly; this Go client exists for DLA only. Returns
// ErrNoRemoteEndpoint unconditionally so the absence of a remote
// endpoint is loud rather than silent.
func (c *Client) TSR(_ context.Context, _ [][]byte) ([][]byte, error) {
	return nil, ErrNoRemoteEndpoint
}
