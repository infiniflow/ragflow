package cli

import (
	"strings"
	"testing"
)

// The single-command mode keeps every global flag out of the command text: a
// leading "-" made the lexer report "expected command, got -" and the command
// never ran.
func TestParseArgsCommandModeStripsGlobalFlags(t *testing.T) {
	cases := []struct {
		args   []string
		format OutputFormat
	}{
		{[]string{"ws", "ls"}, OutputFormatPlain},
		{[]string{"-o", "json", "ws", "ls"}, OutputFormatJSON},
		{[]string{"ws", "ls", "-o", "json"}, OutputFormatJSON},
		{[]string{"-o", "table", "--verbose", "ws", "new", "docs"}, OutputFormatTable},
	}
	for _, tc := range cases {
		cfg, err := ParseArgs(tc.args)
		if err != nil {
			t.Errorf("%v: ParseArgs: %v", tc.args, err)
			continue
		}
		if cfg.CLIMode != CommandMode {
			t.Errorf("%v: mode = %q, want %q", tc.args, cfg.CLIMode, CommandMode)
		}
		if cfg.OutputFormat != tc.format {
			t.Errorf("%v: format = %q, want %q", tc.args, cfg.OutputFormat, tc.format)
		}
		if cfg.Command == nil {
			t.Errorf("%v: no command", tc.args)
			continue
		}
		if strings.Contains(*cfg.Command, "-") {
			t.Errorf("%v: command %q still carries a flag", tc.args, *cfg.Command)
		}
		if _, err := NewParser(*cfg.Command).Parse(CommandMode); err != nil {
			t.Errorf("%v: parse %q: %v", tc.args, *cfg.Command, err)
		}
	}
}

// The connection flags configure the API client instead of joining the
// command text, and without a command the CLI stays interactive.
func TestParseArgsCommandModeConnectionFlags(t *testing.T) {
	cfg, err := ParseArgs([]string{"-h", "10.1.2.3:9380", "-t", "secret", "-u", "alice", "-p", "pw", "ws", "ls"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command == nil || *cfg.Command != "ws ls" {
		t.Fatalf("command = %v, want %q", cfg.Command, "ws ls")
	}
	api := cfg.APIClientConfig.APIServerMap[cfg.APIClientConfig.CurrentAPIServer]
	if api.IP != "10.1.2.3" || api.Port != 9380 {
		t.Errorf("host = %s:%d, want 10.1.2.3:9380", api.IP, api.Port)
	}
	if api.APIKey == nil || *api.APIKey != "secret" {
		t.Errorf("api key not applied")
	}
	if api.UserName == nil || *api.UserName != "alice" {
		t.Errorf("user name not applied")
	}

	cfg, err = ParseArgs([]string{"-h", "10.1.2.3:9380"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command != nil {
		t.Errorf("command = %q, want nil so the CLI stays interactive", *cfg.Command)
	}
	api = cfg.APIClientConfig.APIServerMap[cfg.APIClientConfig.CurrentAPIServer]
	if api.IP != "10.1.2.3" || api.Port != 9380 {
		t.Errorf("host = %s:%d, want 10.1.2.3:9380", api.IP, api.Port)
	}
}

// A reserved word is a valid unquoted one-word question: "retrieve find" must
// ask "find", not fail, while ON keeps its grammar role before the dataset
// clause.
func TestParseAPIRetrieveBareKeywordQuestion(t *testing.T) {
	for _, question := range []string{"hello", "find", "ws", "new", "list", "save"} {
		cmd, err := NewParser(`retrieve ` + question + ` on datasets "kb"`).Parse(APIMode)
		if err != nil {
			t.Errorf("question %q: %v", question, err)
			continue
		}
		if cmd == nil || cmd.Type != "search_on_datasets" {
			t.Errorf("question %q: cmd = %+v", question, cmd)
			continue
		}
		if got := cmd.Params["question"]; got != question {
			t.Errorf("question %q: got %v", question, got)
		}
	}
	if _, err := NewParser(`retrieve on datasets "kb"`).Parse(APIMode); err == nil {
		t.Fatal("a question left empty before ON must not parse")
	}
}

// Workspace names go through the same lexer, so a name may be a reserved word.
func TestSingleWorkspaceNameMayBeReservedWord(t *testing.T) {
	cases := []struct {
		stmt  string
		param string
		value string
	}{
		{"ws use new", "workspace", "new"},
		{"ws new ws", "new_workspace", "ws"},
		{"ws delete find", "workspace", "find"},
		{"ws use date", "workspace", "date"},
		{"ws new features", "new_workspace", "features"},
		{"ws delete groups", "workspace", "groups"},
	}
	for _, tc := range cases {
		cmd, err := NewParser(tc.stmt).Parse(CommandMode)
		if err != nil {
			t.Errorf("%q: %v", tc.stmt, err)
			continue
		}
		if cmd == nil {
			t.Errorf("%q: no command", tc.stmt)
			continue
		}
		if got := cmd.Params[tc.param]; got != tc.value {
			t.Errorf("%q: %s = %v, want %q", tc.stmt, tc.param, got, tc.value)
		}
	}
	if _, err := NewParser(`ws new "docs"`).Parse(CommandMode); err == nil {
		t.Fatal("a quoted string is not a workspace name")
	}
}
