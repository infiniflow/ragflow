package utility

import "testing"

// TestSanitizeFileNameComponent_ControlCharsDeleted is the adversarial test:
// the shared core deletes control characters (<0x20 and 0x7f). This is the
// distinguished behavior from the legacy file_url sanitizeFilename, which
// mapped control chars to '_'. A wrong implementation that maps to '_' fails.
func TestSanitizeFileNameComponent_ControlCharsDeleted(t *testing.T) {
	got := SanitizeFileNameComponent("a\x00b\x1fc\x7f")
	if got != "abc" {
		t.Fatalf("SanitizeFileNameComponent control chars = %q, want %q", got, "abc")
	}
}

func TestSanitizeFileNameComponent_ForbiddenChars(t *testing.T) {
	in := `a/b:c*d?e"f<g>h|i`
	got := SanitizeFileNameComponent(in)
	want := "a_b_c_d_e_f_g_h_i"
	if got != want {
		t.Fatalf("SanitizeFileNameComponent forbidden = %q, want %q", got, want)
	}
}

func TestSanitizeFileNameComponent_PreservesNormalAndUnicode(t *testing.T) {
	if got, want := SanitizeFileNameComponent("报告.pdf"), "报告.pdf"; got != want {
		t.Fatalf("SanitizeFileNameComponent unicode = %q, want %q", got, want)
	}
	if got, want := SanitizeFileNameComponent("plain_name_123"), "plain_name_123"; got != want {
		t.Fatalf("SanitizeFileNameComponent normal = %q, want %q", got, want)
	}
}

func TestSanitizeFileNameComponent_TrimsSpaces(t *testing.T) {
	if got, want := SanitizeFileNameComponent("  name  "), "name"; got != want {
		t.Fatalf("SanitizeFileNameComponent trim = %q, want %q", got, want)
	}
}

func TestSanitizeFileNameComponent_Empty(t *testing.T) {
	if got := SanitizeFileNameComponent(""); got != "" {
		t.Fatalf("SanitizeFileNameComponent empty = %q, want empty", got)
	}
}

func TestSanitizeFileNameComponent_AllForbidden(t *testing.T) {
	if got, want := SanitizeFileNameComponent(`\/:*?"<>|`), "_________"; got != want {
		t.Fatalf("SanitizeFileNameComponent all-forbidden = %q, want %q", got, want)
	}
}
