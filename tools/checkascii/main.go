// Command checkascii rejects non-ASCII bytes on lines added by a pull request.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeader = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)
var frontendTestFile = regexp.MustCompile(`\.(test|spec)\.([jt]sx?|[cm][jt]s)$`)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: checkascii <base-ref> <head-ref>")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := check(ctx, ".", os.Args[1], os.Args[2], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(ctx context.Context, dir, base, head string, out io.Writer) error {
	// Resolve refs before constructing the range so invalid refs fail closed.
	refs := []string{base, head}
	for i, ref := range refs {
		commit, err := gitOutput(ctx, dir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
		if err != nil {
			return err
		}
		refs[i] = strings.TrimSpace(string(commit))
	}
	prRange := refs[0] + "..." + refs[1]
	files, err := gitOutput(ctx, dir, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--diff-filter=AMT", "--name-only", "-z", prRange, "--")
	if err != nil {
		return err
	}
	failures := 0
	escape := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	for _, name := range bytes.Split(files, []byte{0}) {
		file := string(name)
		if file == "" || isExcluded(file) {
			continue
		}
		// Inspect committed PR content, independent of the checked-out merge tree.
		// Git omits binary patches; renames are additions so moving a file cannot
		// bypass the policy by moving it out of an excluded directory.
		diff, err := gitOutput(ctx, dir, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", "--unified=0", prRange, "--", file)
		if err != nil {
			return err
		}
		lineNumber := 0
		for _, line := range bytes.Split(diff, []byte{'\n'}) {
			if match := hunkHeader.FindSubmatch(line); match != nil {
				lineNumber, err = strconv.Atoi(string(match[1]))
				if err != nil {
					return fmt.Errorf("parse diff line number for %q: %w", file, err)
				}
				continue
			}
			if lineNumber == 0 || len(line) == 0 {
				continue
			}
			switch line[0] {
			case '+':
				for _, b := range line[1:] {
					if b >= 0x80 {
						if _, err := fmt.Fprintf(out, "::error file=%s,line=%d::Non-ASCII characters on added line. Use English ASCII text or Unicode escapes in string literals.\n", escape.Replace(file), lineNumber); err != nil {
							return err
						}
						failures++
						break
					}
				}
				lineNumber++
			case ' ':
				lineNumber++
			}
		}
	}
	if failures > 0 {
		return fmt.Errorf("non-ASCII check failed: %d added line(s); see tools/checkascii/README.md for exclusions", failures)
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--literal-pathspecs"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func isExcluded(file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return true
	}
	if strings.HasPrefix(file, "web/") && (frontendTestFile.MatchString(file) || strings.Contains(file, "/__tests__/") || strings.Contains(file, "/__mocks__/") || file == "web/jest-setup.ts") {
		return true
	}
	if path.Dir(file) == "web/src/locales" && path.Ext(file) == ".ts" {
		return path.Base(file) != "en.ts"
	}
	if path.Dir(file) == "." && strings.HasPrefix(file, "README_") && strings.HasSuffix(file, ".md") {
		return true
	}
	if path.Dir(file) == "internal/agent/templates" && path.Ext(file) == ".json" {
		return true
	}
	if file == "internal/rag/res/ner.json" || file == "internal/rag/res/synonym.json" {
		return true
	}
	// Fixture data needs literal multilingual text. Source code in these
	// directories is still checked unless it matches a test exclusion above.
	switch path.Ext(file) {
	case ".json", ".txt", ".md", ".html", ".csv", ".tsv", ".xml", ".svg", ".b64":
		for _, part := range strings.Split(path.Dir(file), "/") {
			if part == "testdata" || part == "fixtures" || part == "golden" {
				return true
			}
		}
	}
	return false
}
