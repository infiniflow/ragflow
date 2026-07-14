package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"ragflow/internal/ingestion/task"
)

func main() {
	caseDir := flag.String("case-dir", "", "case directory under internal/ingestion/task/testdata/<case_id>")
	docID := flag.String("doc-id", "doc-1", "document id")
	kbID := flag.String("kb-id", "kb-1", "knowledge base id")
	docName := flag.String("doc-name", "sample.md", "document name")
	flag.Parse()

	if *caseDir == "" {
		exitErr(errors.New("--case-dir is required"))
	}

	inputPath := filepath.Join(*caseDir, "input.json")
	outputDir := filepath.Join(*caseDir, "output")

	input := mustReadJSONMap(inputPath)

	expectedNormalized := mustReadJSONSlice(filepath.Join(outputDir, "normalized_chunks.json"))
	expectedProcessed := mustReadJSONSlice(filepath.Join(outputDir, "processed_chunks.json"))
	expectedMetadata := mustReadJSONMap(filepath.Join(outputDir, "merged_metadata.json"))
	expectedProcessError, hasExpectedProcessError := readOptionalJSONMap(filepath.Join(outputDir, "process_error.json"))

	actual, actualErr := runActual(input, *docID, *kbID, *docName)
	if hasExpectedProcessError || actualErr != nil {
		reportProcessErrorOutcome(expectedProcessError, hasExpectedProcessError, actualErr)
		return
	}

	diffs := make([]string, 0)
	diffs = append(diffs, diffValue("normalized_chunks", sanitizeNormalized(expectedNormalized), sanitizeNormalized(actual.NormalizedChunks), false)...)
	diffs = append(diffs, diffValue("processed_chunks", sanitizeProcessed(expectedProcessed), sanitizeProcessed(actual.ProcessedChunks), true)...)
	diffs = append(diffs, diffValue("merged_metadata", expectedMetadata, actual.MergedMetadata, false)...)

	if len(diffs) == 0 {
		fmt.Println("validation succeeded")
		return
	}

	fmt.Println("validation failed")
	for _, d := range diffs {
		fmt.Println(d)
	}
	os.Exit(1)
}

func runActual(input map[string]any, docID string, kbID string, docName string) (result task.GoldenCompareResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return task.ProcessPipelineOutputForGolden(input, docID, kbID, docName), nil
}

func reportProcessErrorOutcome(expected map[string]any, hasExpected bool, actualErr error) {
	switch {
	case hasExpected && actualErr == nil:
		fmt.Println("validation failed")
		fmt.Printf("expected process error: %v\n", expected)
		fmt.Println("actual: processing succeeded")
		os.Exit(1)
	case !hasExpected && actualErr != nil:
		fmt.Println("validation failed")
		fmt.Println("expected: processing succeeded")
		fmt.Printf("actual error: %v\n", actualErr)
		os.Exit(1)
	case hasExpected && actualErr != nil:
		expectedMsg, _ := expected["message"].(string)
		if expectedMsg != "" && strings.Contains(actualErr.Error(), expectedMsg) {
			fmt.Println("validation succeeded")
			fmt.Printf("expected process error observed: %s\n", expectedMsg)
			return
		}
		fmt.Println("validation failed")
		fmt.Printf("expected process error: %v\n", expected)
		fmt.Printf("actual error: %v\n", actualErr)
		os.Exit(1)
	default:
		return
	}
}

func sanitizeProcessed(chunks []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		cp := normalizeComparableMap(cloneMap(chunk))
		delete(cp, "create_time")
		delete(cp, "create_timestamp_flt")
		for k := range cp {
			if strings.HasPrefix(k, "q_") && strings.HasSuffix(k, "_vec") {
				delete(cp, k)
			}
		}
		out = append(out, cp)
	}
	return out
}

func sanitizeNormalized(chunks []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, normalizeComparableMap(cloneMap(chunk)))
	}
	return out
}

func diffValue(path string, expected any, actual any, sortMaps bool) []string {
	if reflect.DeepEqual(expected, actual) {
		return nil
	}
	return walkDiff(path, expected, actual, sortMaps)
}

func walkDiff(path string, expected any, actual any, sortMaps bool) []string {
	switch exp := expected.(type) {
	case map[string]any:
		act, ok := actual.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: expected map, got %T", path, actual)}
		}
		keys := unionKeys(exp, act)
		diffs := make([]string, 0)
		for _, key := range keys {
			ev, eok := exp[key]
			av, aok := act[key]
			switch {
			case !eok:
				diffs = append(diffs, fmt.Sprintf("%s.%s: unexpected actual value %v", path, key, av))
			case !aok:
				diffs = append(diffs, fmt.Sprintf("%s.%s: missing actual value, expected %v", path, key, ev))
			default:
				diffs = append(diffs, walkDiff(path+"."+key, ev, av, sortMaps)...)
			}
		}
		return diffs
	case []any:
		act, ok := actual.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: expected []any, got %T", path, actual)}
		}
		return diffSlice(path, exp, act, sortMaps)
	case []map[string]any:
		act, ok := actual.([]map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: expected []map[string]any, got %T", path, actual)}
		}
		diffs := make([]string, 0)
		if len(exp) != len(act) {
			diffs = append(diffs, fmt.Sprintf("%s: len mismatch expected=%d actual=%d", path, len(exp), len(act)))
		}
		n := min(len(exp), len(act))
		for i := 0; i < n; i++ {
			diffs = append(diffs, walkDiff(fmt.Sprintf("%s[%d]", path, i), exp[i], act[i], sortMaps)...)
		}
		return diffs
	default:
		if !reflect.DeepEqual(expected, actual) {
			return []string{fmt.Sprintf("%s: expected=%v (%T), actual=%v (%T)", path, expected, expected, actual, actual)}
		}
		return nil
	}
}

func diffSlice(path string, expected []any, actual []any, sortMaps bool) []string {
	diffs := make([]string, 0)
	if len(expected) != len(actual) {
		diffs = append(diffs, fmt.Sprintf("%s: len mismatch expected=%d actual=%d", path, len(expected), len(actual)))
	}
	n := min(len(expected), len(actual))
	for i := 0; i < n; i++ {
		diffs = append(diffs, walkDiff(fmt.Sprintf("%s[%d]", path, i), expected[i], actual[i], sortMaps)...)
	}
	return diffs
}

func unionKeys(a map[string]any, b map[string]any) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeComparableMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = normalizeComparableValue(v)
	}
	return out
}

func normalizeComparableValue(v any) any {
	switch val := v.(type) {
	case []string:
		out := make([]any, 0, len(val))
		for _, item := range val {
			out = append(out, item)
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(val))
		for _, item := range val {
			out = append(out, normalizeComparableMap(item))
		}
		return out
	case map[string]any:
		return normalizeComparableMap(val)
	default:
		return v
	}
}

func mustReadJSONMap(path string) map[string]any {
	var out map[string]any
	mustReadJSON(path, &out)
	return out
}

func mustReadJSONSlice(path string) []map[string]any {
	var out []map[string]any
	mustReadJSON(path, &out)
	return out
}

func mustReadJSON(path string, out any) {
	data, err := os.ReadFile(path)
	if err != nil {
		exitErr(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		exitErr(fmt.Errorf("unmarshal %s: %w", path, err))
	}
}

func readOptionalJSONMap(path string) (map[string]any, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		exitErr(fmt.Errorf("unmarshal %s: %w", path, err))
	}
	return out, true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
