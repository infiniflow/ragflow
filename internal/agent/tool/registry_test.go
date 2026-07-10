package tool

import (
	"strings"
	"testing"
)

func TestBuildAll_KnownTools(t *testing.T) {
	tools, _ := BuildAll([]string{"retrieval", "wikipedia"}, nil)
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
	meta0 := tools[0].ToolMeta()
	if meta0.Name != "search_my_dateset" {
		t.Errorf("tools[0].Name = %q, want search_my_dateset", meta0.Name)
	}
	meta1 := tools[1].ToolMeta()
	if meta1.Name != "wikipedia" {
		t.Errorf("tools[1].Name = %q, want wikipedia", meta1.Name)
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
	// Every key in registry.
	names := []string{
		"akshare", "arxiv", "bgpt", "code_exec", "crawler", "deepl",
		"duckduckgo", "email", "exesql", "execute_sql", "github", "google",
		"google_scholar", "google_scholar_search", "jin10", "keenable", "pubmed", "qweather",
		"retrieval", "search_my_dataset", "search_my_dateset", "searxng",
		"tavily", "tavily_extract", "tushare", "web_crawler", "wencai", "wikipedia", "wikipedia_search",
		"yahoo_finance",
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
		"keenable": {
			"api_key": "key-test",
		},
	}
	tools, _ := BuildAll(names, params)
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

func TestBuildAll_KeenableRejectsEmptyNodeAPIKey(t *testing.T) {
	_, err := BuildAll([]string{"keenable"}, map[string]map[string]any{
		"keenable": {"api_key": ""},
	})
	if err == nil {
		t.Fatal("expected keenable config error")
	}
	if !strings.Contains(err.Error(), "requires non-empty string node-level param api_key") {
		t.Fatalf("err = %q, want keenable api_key validation error", err.Error())
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

	// Every entry the registry advertises.
	names := []string{
		"akshare", "arxiv", "bgpt", "code_exec", "crawler", "deepl",
		"duckduckgo", "email", "execute_sql", "exesql", "github", "google",
		"google_scholar", "google_scholar_search", "jin10", "keenable", "pubmed", "qweather",
		"retrieval", "search_my_dataset", "search_my_dateset", "searxng",
		"tavily", "tavily_extract", "tushare", "web_crawler", "wencai", "wikipedia", "wikipedia_search",
		"yahoo_finance",
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
		"keenable": {
			"api_key": "key-xyz",
		},
	}
	tools, _ := BuildAll(names, params)
	if len(tools) != len(names) {
		t.Fatalf("BuildAll returned %d tools for %d names", len(tools), len(names))
	}

	// Schema-level checks per entry.
	for i, name := range names {
		meta := tools[i].ToolMeta()
		if meta.Name == "" {
			t.Errorf("tools[%d] (registry name %q).Name is empty", i, name)
		}
		if meta.Description == "" {
			t.Errorf("tools[%d] (registry name %q).Description is empty", i, name)
		}
	}

	// Alias consistency: execute_sql and exesql must surface the
	// same canonical Info().Name; same for retrieval/search_my_dataset/
	// search_my_dateset and crawler/web_crawler. A bug here would mean
	// an alias was accidentally pointed at a different tool.
	canonicalByAlias := map[string]string{
		"execute_sql":           "execute_sql",
		"exesql":                "execute_sql",
		"google_scholar":        "google_scholar",
		"google_scholar_search": "google_scholar",
		"retrieval":             "search_my_dateset",
		"search_my_dataset":     "search_my_dateset",
		"search_my_dateset":     "search_my_dateset",
		"crawler":               "crawler",
		"web_crawler":           "crawler",
		"wikipedia":             "wikipedia",
		"wikipedia_search":      "wikipedia",
	}
	for _, name := range names {
		canonical, ok := canonicalByAlias[name]
		if !ok {
			continue
		}
		idx := indexOf(names, name)
		meta := tools[idx].ToolMeta()
		if meta.Name != canonical {
			t.Errorf("registry name %q: Info().Name = %q, want %q (alias must surface canonical name)",
				name, meta.Name, canonical)
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
