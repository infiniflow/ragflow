package esoscommon

import (
	"cmp"
	"context"
	"errors"
	"net"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// ChunkID returns the first non-empty chunk identifier from "id" or "_id".
func ChunkID(chunk map[string]interface{}) (string, bool) {
	if id, ok := chunk["id"].(string); ok && id != "" {
		return id, true
	}
	if id, ok := chunk["_id"].(string); ok && id != "" {
		return id, true
	}
	return "", false
}

// CountAggregationTag increments the count for a non-blank tag.
func CountAggregationTag(counts map[string]int, tag string) {
	if tag = strings.TrimSpace(tag); tag != "" {
		counts[tag]++
	}
}

// CompileHighlightPattern builds a case-insensitive regex from keywords,
// ordered by descending rune length. Returns nil when no keyword is present.
func CompileHighlightPattern(keywords []string) *regexp.Regexp {
	nonEmpty := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		if kw != "" {
			nonEmpty = append(nonEmpty, kw)
		}
	}
	if len(nonEmpty) == 0 {
		return nil
	}
	slices.SortStableFunc(nonEmpty, func(a, b string) int {
		return cmp.Compare(utf8.RuneCountInString(b), utf8.RuneCountInString(a))
	})
	parts := make([]string, len(nonEmpty))
	for i, keyword := range nonEmpty {
		parts[i] = regexp.QuoteMeta(keyword)
		if IsLatinKeyword(keyword) {
			parts[i] += `\p{Latin}*`
		}
	}
	return regexp.MustCompile("(?i)" + strings.Join(parts, "|"))
}

// IsTimeoutError detects connection-level and per-attempt timeouts
// via context.DeadlineExceeded, net.Error.Timeout(), and substring
// matches (for SDKs that wrap without typed errors).
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	for _, sub := range []string{"i/o timeout", "deadline exceeded", "connection timeout", "context deadline"} {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}
