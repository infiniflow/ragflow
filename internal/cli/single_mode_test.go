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

// Every single-mode command the parser produces must reach a handler: a type
// the dispatch switch does not know fails with "single command mode: <type>".
func TestSingleCommandDispatchCoversParserTypes(t *testing.T) {
	statements := []string{
		"ws ls", "ws cat f", "ws search q", "ws find q", "ws",
		"ws use docs", "ws new docs", "ws delete docs", "ws save hi",
	}
	for _, stmt := range statements {
		cmd, err := NewParser(stmt).Parse(CommandMode)
		if err != nil {
			t.Errorf("%q: %v", stmt, err)
			continue
		}
		c := &CLI{Config: &CommandLineConfig{OutputFormat: OutputFormatPlain}}
		if _, err := c.ExecuteSingleCommand(1, cmd); err != nil {
			t.Errorf("%q (%s): %v", stmt, cmd.Type, err)
		}
	}
}

// The connection flags configure the API client instead of joining the
// command text, and without a command the CLI stays interactive.
// Single-command mode takes its connection settings from rf.yml alone, so the
// connection and config flags are rejected instead of configuring the client
// or landing in the command text.
func TestParseArgsCommandModeRejectsConnectionFlags(t *testing.T) {
	flags := [][]string{
		{"-h", "10.1.2.3:9380"},
		{"--host", "10.1.2.3:9380"},
		{"-t", "secret"},
		{"--token", "secret"},
		{"-u", "alice"},
		{"--user", "alice"},
		{"-p", "pw"},
		{"--password", "pw"},
		{"-f", "other.yml"},
		{"--config", "other.yml"},
		{"-k", "key.pem"},
		{"--key", "key.pem"},
	}
	for _, flag := range flags {
		args := append(append([]string{}, flag...), "ws", "ls")
		_, err := ParseArgs(args)
		if err == nil || !strings.Contains(err.Error(), "rf.yml") {
			t.Errorf("%v: err = %v, want a rejection naming rf.yml", args, err)
		}
	}
}

// Without the connection flags the client keeps the rf.yml defaults, and the
// command text still loses the output switches.
func TestParseArgsCommandModeUsesConfigDefaults(t *testing.T) {
	cfg, err := ParseArgs([]string{"ws", "ls"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command == nil || *cfg.Command != "ws ls" {
		t.Fatalf("command = %v, want %q", cfg.Command, "ws ls")
	}
	api := cfg.APIClientConfig.APIServerMap[cfg.APIClientConfig.CurrentAPIServer]
	if api.IP != "127.0.0.1" || api.Port != 9380 {
		t.Errorf("host = %s:%d, want the rf.yml defaults 127.0.0.1:9380", api.IP, api.Port)
	}
}

// Once a command word starts the command, every later token belongs to the
// command grammar, which has its own flags: "search -h" prints the search
// options, so a flag-looking token there is not a connection flag to reject.
func TestParseArgsCommandModeKeepsCommandFlags(t *testing.T) {
	cfg, err := ParseArgs([]string{"search", "-h"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command == nil || *cfg.Command != "search -h" {
		t.Fatalf("command = %v, want %q", cfg.Command, "search -h")
	}
	if _, err := NewParser(*cfg.Command).Parse(CommandMode); err != nil {
		t.Fatalf("parse %q: %v", *cfg.Command, err)
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
