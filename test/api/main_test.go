// tests/api/main_test.go
////go:build integration

package api

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

var baseURL string

var priority = flag.String("priority", "p1", "test priority: p1 | p2 | p3")
var TestPriority int

func TestMain(m *testing.M) {
	fmt.Fprintln(os.Stderr, "TearUp")

	flag.Parse()
	switch *priority {
	case "p0", "p1", "p2", "p3":
		priorityInt, err := ParsePriorityString(*priority)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid priority: %s\n", *priority)
			os.Exit(-1)
		}

		TestPriority = priorityInt
	default:
		os.Exit(-1)
	}

	// Run the test cases
	code := m.Run()

	fmt.Fprintln(os.Stderr, "TearDown")
	os.Exit(code)
}
