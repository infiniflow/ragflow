package knowledge_compile

import (
	"os"
	"testing"
)

// TestKCCompileConcurrencyOverride locks the KC_COMPILE_CONCURRENCY env
// override: unset / zero / negative / non-numeric all mean "no override" (0),
// and a positive value is returned as-is. Mirrors TestStructureBatchCap.
func TestKCCompileConcurrencyOverride(t *testing.T) {
	const key = "KC_COMPILE_CONCURRENCY"
	cases := []struct {
		name string
		set  string // "" means unset
		want int
	}{
		{"unset", "", 0},
		{"zero", "0", 0},
		{"negative", "-2", 0},
		{"non-numeric", "xyz", 0},
		{"positive", "8", 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orig, had := os.LookupEnv(key)
			if c.set == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, c.set)
			}
			t.Cleanup(func() {
				if had {
					os.Setenv(key, orig)
				} else {
					os.Unsetenv(key)
				}
			})
			if got := kcCompileConcurrencyOverride(); got != c.want {
				t.Fatalf("kcCompileConcurrencyOverride() = %d, want %d (env=%q)", got, c.want, c.set)
			}
		})
	}
}
