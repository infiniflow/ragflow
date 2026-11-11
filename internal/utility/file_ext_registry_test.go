// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utility

import "testing"

// resetExtensionFileTypes isolates the registry for a test: it saves the
// current map (which may hold edition registrations from init()) and replaces
// it with an empty one, returning a restore func so those registrations come
// back after the test.
func resetExtensionFileTypes() func() {
	saved := extensionFileTypes
	extensionFileTypes = map[string]FileType{}
	return func() { extensionFileTypes = saved }
}

// EE editions register parser types the community build does not know about.
// GetFileType must return the registered type for that extension.
func TestGetFileTypeConsultsRegisteredExtension(t *testing.T) {
	defer resetExtensionFileTypes()()
	const cad FileType = "cad"
	RegisterExtensionFileType("cad", cad)
	if got := GetFileType("report.cad"); got != cad {
		t.Fatalf("GetFileType(\"report.cad\") = %q, want %q", got, cad)
	}
}

// Without a registration the community behaviour is unchanged.
func TestGetFileTypeUnknownExtensionStillOther(t *testing.T) {
	defer resetExtensionFileTypes()()
	if got := GetFileType("report.cad"); got != FileTypeOTHER {
		t.Fatalf("GetFileType(\"report.cad\") = %q, want FileTypeOTHER (unregistered)", got)
	}
}

// Registering one extension must not leak into an unrelated unregistered one.
func TestGetFileTypeRegisteredExtensionDoesNotAffectUnregistered(t *testing.T) {
	defer resetExtensionFileTypes()()
	RegisterExtensionFileType("cad", FileType("cad"))
	if got := GetFileType("model.dwg"); got != FileTypeOTHER {
		t.Fatalf("GetFileType(\"model.dwg\") = %q, want FileTypeOTHER", got)
	}
}

// Extension registration is case-insensitive on the suffix.
func TestGetFileTypeExtensionCaseInsensitive(t *testing.T) {
	defer resetExtensionFileTypes()()
	RegisterExtensionFileType("CAD", FileType("cad"))
	if got := GetFileType("model.cad"); got != FileType("cad") {
		t.Fatalf("GetFileType(\"model.cad\") with upper-registered suffix = %q, want %q", got, FileType("cad"))
	}
}

// An empty suffix registration is a no-op and never panics.
func TestGetFileTypeEmptySuffixIgnored(t *testing.T) {
	defer resetExtensionFileTypes()()
	RegisterExtensionFileType("", FileType("cad"))
	if got := GetFileType("report.cad"); got != FileTypeOTHER {
		t.Fatalf("RegisterExtensionFileType(\"\") should be ignored; GetFileType = %q, want FileTypeOTHER", got)
	}
}

// The upload gate (FilenameType) must also admit registered extensions.
func TestFilenameTypeConsultsRegisteredExtension(t *testing.T) {
	defer resetExtensionFileTypes()()
	RegisterExtensionFileType("cad", FileType("cad"))
	if got := FilenameType("report.cad"); got != FileType("cad") {
		t.Fatalf("FilenameType(\"report.cad\") = %q, want %q", got, FileType("cad"))
	}
}

// A bare-dot registration must normalize to an empty suffix and be ignored,
// so it cannot poison every extensionless filename via an empty map key.
func TestRegisterExtensionFileTypeDotOnlyIgnored(t *testing.T) {
	defer resetExtensionFileTypes()()
	RegisterExtensionFileType(".", FileType("dot"))
	if got := GetFileType("report"); got != FileTypeOTHER {
		t.Fatalf("GetFileType(\"report\") after \".\" registration = %q, want FileTypeOTHER", got)
	}
	if got := GetFileType("report."); got != FileTypeOTHER {
		t.Fatalf("GetFileType(\"report.\") after \".\" registration = %q, want FileTypeOTHER", got)
	}
}

// A registered override of a built-in extension must win in FilenameType too,
// keeping it consistent with GetFileType (the redirect case).
func TestFilenameTypeHonorsRegisteredOverrideForBuiltin(t *testing.T) {
	defer resetExtensionFileTypes()()
	RegisterExtensionFileType("pdf", FileType("mypdf"))
	if got := FilenameType("report.pdf"); got != FileType("mypdf") {
		t.Fatalf("FilenameType(\"report.pdf\") = %q, want mypdf (registered override)", got)
	}
}

// When nothing is registered for a built-in extension, FilenameType keeps its
// community upload-bucket behavior.
func TestFilenameTypeBuiltinPdfUnregistered(t *testing.T) {
	defer resetExtensionFileTypes()()
	if got := FilenameType("report.pdf"); got != FileTypePDF {
		t.Fatalf("FilenameType(\"report.pdf\") unregistered = %q, want FileTypePDF", got)
	}
}

// resetExtensionFileTypes must preserve registrations that existed before the
// test (e.g. edition registrations installed by init()), so later tests in the
// same binary still see them after this test's isolation ends.
func TestResetExtensionFileTypesRestoresPriorRegistration(t *testing.T) {
	const baseline FileType = "baseline"
	RegisterExtensionFileType("baseline", baseline)

	restore := resetExtensionFileTypes()
	if _, ok := extensionFileTypes["baseline"]; ok {
		restore()
		t.Fatal("baseline should be gone immediately after reset")
	}
	RegisterExtensionFileType("tmp", FileType("tmp"))

	restore()

	if got := extensionFileTypes["baseline"]; got != baseline {
		t.Fatalf("baseline should be restored after restore(); got %q", got)
	}
	if _, ok := extensionFileTypes["tmp"]; ok {
		t.Fatal("tmp should be gone after restore()")
	}
}
