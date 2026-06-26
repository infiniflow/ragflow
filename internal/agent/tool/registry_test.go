package tool

import (
	"context"
	"strings"
	"testing"
)

func TestBuildAll_KnownTools(t *testing.T) {
	tools, err := BuildAll([]string{"retrieval", "wikipedia"}, nil)
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
	info0, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tools[0].Info: %v", err)
	}
	if info0.Name != "search_my_dateset" {
		t.Errorf("tools[0].Info().Name = %q, want search_my_dateset", info0.Name)
	}
	info1, err := tools[1].Info(context.Background())
	if err != nil {
		t.Fatalf("tools[1].Info: %v", err)
	}
	if info1.Name != "wikipedia" {
		t.Errorf("tools[1].Info().Name = %q, want wikipedia", info1.Name)
	}
}

func TestBuildAll_UnknownTool(t *testing.T) {
	_, err := BuildAll([]string{"does_not_exist"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), `unsupported tool "does_not_exist"`) {
		t.Fatalf("err = %q, want unsupported tool message", err.Error())
	}
}

func TestBuildAll_AllRegisteredTools(t *testing.T) {
	names := []string{
		"akshare", "arxiv", "code_exec", "crawler", "deepl", "duckduckgo",
		"email", "github", "google", "google_scholar", "jin10", "pubmed",
		"qweather", "retrieval", "searxng", "tavily", "tushare", "wencai",
		"wikipedia", "yahoo_finance", "execute_sql",
	}
	params := map[string]map[string]any{
		"execute_sql": {
			"db_type":   "mysql",
			"host":      "127.0.0.1",
			"port":      3306,
			"database":  "demo",
			"username":  "u",
			"password":  "p",
			"max_records": 10,
		},
	}
	tools, err := BuildAll(names, params)
	if err != nil {
		t.Fatalf("BuildAll(all registered): %v", err)
	}
	if len(tools) != len(names) {
		t.Fatalf("len(tools) = %d, want %d", len(tools), len(names))
	}
}

func TestBuildAll_ExeSQLRequiresNodeParams(t *testing.T) {
	_, err := BuildAll([]string{"execute_sql"}, nil)
	if err == nil {
		t.Fatal("expected execute_sql config error")
	}
	if !strings.Contains(err.Error(), "execute_sql requires node-level params") {
		t.Fatalf("err = %q, want execute_sql config error", err.Error())
	}
}

// TestToolRegistry_SchemasAreComplete sweeps every name the public
// registry advertises (including the execute_sql/exesql and
// retrieval/search_my_dateset alias pairs), builds the tool, and
// asserts that its Info() returns a complete schema — non-empty
// Name and Desc, non-nil ParamsOneOf, and a consistent canonical
// name across alias entries. Catches drift like "tool renamed but
// registry not updated", "param added but schema not updated",
// "tool registered with empty description", and "alias points to
// the wrong canonical name".
func TestToolRegistry_SchemasAreComplete(t *testing.T) {
	t.Parallel()

	// Every entry the registry advertises. 23 names, 22 unique
	// canonical tools (execute_sql == exesql, retrieval ==
	// search_my_dateset).
	names := []string{
		"akshare", "arxiv", "code_exec", "crawler", "deepl", "duckduckgo",
		"email", "execute_sql", "exesql", "github", "google",
		"google_scholar", "jin10", "pubmed", "qweather", "retrieval",
		"search_my_dateset", "searxng", "tavily", "tushare", "wencai",
		"wikipedia", "yahoo_finance",
	}
	params := map[string]map[string]any{
		"execute_sql": {
			"db_type":     "mysql",
			"host":        "127.0.0.1",
			"port":        3306,
			"database":    "demo",
			"username":    "u",
			"password":    "p",
			"max_records": 10,
		},
		"exesql": {
			"db_type":     "mysql",
			"host":        "127.0.0.1",
			"port":        3306,
			"database":    "demo",
			"username":    "u",
			"password":    "p",
			"max_records": 10,
		},
	}
	tools, err := BuildAll(names, params)
	if err != nil {
		t.Fatalf("BuildAll(%d names): %v", len(names), err)
	}
	if len(tools) != len(names) {
		t.Fatalf("BuildAll returned %d tools for %d names", len(tools), len(names))
	}

	// Schema-level checks per entry.
	for i, name := range names {
		info, err := tools[i].Info(context.Background())
		if err != nil {
			t.Errorf("tools[%d] (registry name %q).Info: %v", i, name, err)
			continue
		}
		if info.Name == "" {
			t.Errorf("tools[%d] (registry name %q).Info().Name is empty", i, name)
		}
		if info.Desc == "" {
			t.Errorf("tools[%d] (registry name %q).Info().Desc is empty", i, name)
		}
		if info.ParamsOneOf == nil {
			t.Errorf("tools[%d] (registry name %q).Info().ParamsOneOf is nil", i, name)
		}
	}

	// Alias consistency: execute_sql and exesql must surface the
	// same canonical Info().Name; same for retrieval and
	// search_my_dateset. A bug here would mean an alias was
	// accidentally pointed at a different tool.
	canonicalByAlias := map[string]string{
		"execute_sql":      "execute_sql",
		"exesql":           "execute_sql",
		"retrieval":        "search_my_dateset",
		"search_my_dateset": "search_my_dateset",
	}
	for _, name := range names {
		canonical, ok := canonicalByAlias[name]
		if !ok {
			continue
		}
		idx := indexOf(names, name)
		info, err := tools[idx].Info(context.Background())
		if err != nil {
			continue
		}
		if info.Name != canonical {
			t.Errorf("registry name %q: Info().Name = %q, want %q (alias must surface canonical name)",
				name, info.Name, canonical)
		}
	}
}

// indexOf returns the index of s in xs, or -1 if not present.
// Tiny helper to keep the alias loop above free of a slice lookup
// closure; the test's names slice is <30 items so linear scan is
// fine.
func indexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}
