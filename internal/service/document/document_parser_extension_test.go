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

package document

import (
	"testing"

	"ragflow/internal/utility"
)

func TestDedicatedDocumentParserRegisteredPresentationExtension(t *testing.T) {
	utility.RegisterExtensionFileType("officedeck", utility.FileTypePPT)
	t.Cleanup(func() { utility.RegisterExtensionFileType("officedeck", utility.FileTypeOTHER) })
	if got := dedicatedDocumentParserID(utility.FileTypeDOC, "sample.officedeck"); got != "presentation" {
		t.Fatalf("dedicated parser = %q, want presentation", got)
	}
}
