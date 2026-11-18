package agentic_rag

import (
	"testing"

	"ragflow/internal/common"
)

// TestAgenticMaxIterationsResolution pins the precedence of the loop cap: an
// explicit request, then the environment, then the compiled default.
//
// The default case is the one that matters most: leaving the variable unset
// must reproduce the old behaviour exactly (120), because every measurement
// before the cap existed was taken at 120, and a silent change of default would
// make those runs incomparable.
func TestAgenticMaxIterationsResolution(t *testing.T) {
	cases := []struct {
		name     string
		explicit int
		env      string
		setEnv   bool
		want     int
	}{
		{name: "nothing set keeps the compiled default", want: defaultMaxIterations},
		{name: "env lowers the cap", env: "16", setEnv: true, want: 16},
		{name: "env raises it too", env: "200", setEnv: true, want: 200},
		{name: "explicit input wins over env", explicit: 8, env: "32", setEnv: true, want: 8},
		{name: "zero env value means unset", env: "0", setEnv: true, want: defaultMaxIterations},
		{name: "negative env falls back", env: "-5", setEnv: true, want: defaultMaxIterations},
		{name: "unparsable env falls back", env: "sixteen", setEnv: true, want: defaultMaxIterations},
		{name: "explicit one is honoured", explicit: 1, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				t.Setenv(common.EnvAgenticMaxIterations, tc.env)
			}
			if got := agenticMaxIterations(tc.explicit); got != tc.want {
				t.Fatalf("agenticMaxIterations(%d) = %d, want %d", tc.explicit, got, tc.want)
			}
		})
	}
}
