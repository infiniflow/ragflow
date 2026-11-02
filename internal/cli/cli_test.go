//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package cli

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestSanitizeCLIError_StripsSingleQuotedUserInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   error
		want string
	}{
		{
			name: "dataset name in single quotes",
			in:   fmt.Errorf("dataset '%s' not found", "secret-project-name"),
			want: "dataset not found",
		},
		{
			name: "file path in single quotes",
			in:   fmt.Errorf("file '%s' has bad content", "/home/user/.ssh/id_rsa"),
			want: "file has bad content",
		},
		{
			name: "no quoted content passes through",
			in:   errors.New("connection refused"),
			want: "connection refused",
		},
		{
			name: "nil error returns empty string",
			in:   nil,
			want: "",
		},
		{
			name: "empty string after stripping",
			in:   errors.New("'everything-stripped'"),
			want: "command failed",
		},
		{
			name: "two quoted paths in one error",
			in:   fmt.Errorf("copy '%s' to '%s' failed", "/secret/a", "/secret/b"),
			want: "copy to failed",
		},
		{
			name: "three quoted values mixed with text — only the sensitive spans are stripped",
			in:   fmt.Errorf("'%s' is not a valid %s in %s", "secret-name", "kind", "scope"),
			want: "is not a valid kind in scope",
		},
		{
			name: "unmatched single quote is preserved",
			in:   errors.New("oops 'unterminated"),
			want: "oops 'unterminated",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeCLIError(tc.in); got != tc.want {
				t.Errorf("sanitizeCLIError(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Every meta-command and user-mode statement listed by \help must be accepted
// as written, without dropping any of its quoted arguments, so users can copy
// commands from the help.
func TestHelpListsAcceptedCommands(t *testing.T) {
	userCommands := registeredUserCommands(t)
	section := ""
	for _, line := range strings.Split(helpText, "\n") {
		if line != "" && !strings.HasPrefix(line, " ") {
			section = line
			continue
		}
		entry := strings.TrimSpace(line)
		switch section {
		case "Meta Commands:":
			names, _, ok := strings.Cut(entry, " - ")
			if !ok {
				continue
			}
			for _, name := range strings.Split(names, " or ") {
				name = strings.TrimSpace(name)
				cmd, err := NewParser(name).Parse(APIMode)
				if err != nil || cmd == nil || cmd.Type != "meta" {
					t.Errorf("%s: parsed as %v, err = %v", name, cmd, err)
					continue
				}
				c := &CLI{running: true}
				if err := c.handleMetaCommand(1, cmd); err != nil {
					t.Errorf("%s: %v", name, err)
				}
			}
		case "Commands (User Mode):":
			stmt, _, ok := strings.Cut(entry, ";")
			if !ok {
				continue
			}
			stmt = strings.ReplaceAll(stmt, " [options]", "") + ";"
			p := NewParser(stmt)
			cmd, err := p.Parse(APIMode)
			if err != nil {
				t.Errorf("%s: %v", stmt, err)
				continue
			}
			if cmd == nil || !userCommands[cmd.Type] {
				t.Errorf("%s: no ExecuteUserCommand handler for %v", stmt, cmd)
			}
			for ; p.curToken.Type != TokenEOF; p.nextToken() {
				if p.curToken.Type == TokenQuotedString ||
					(strings.HasPrefix(stmt, "SHOW CURRENT") && isKeyword(p.curToken.Type)) {
					t.Errorf("%s: argument %q is ignored", stmt, p.curToken.Value)
					break
				}
			}
		}
	}
}

// Read the dispatcher's cases rather than duplicating its command list or
// invoking handlers that can contact a server or change CLI state.
func registeredUserCommands(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "cli_http.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "ExecuteUserCommand" {
			continue
		}
		for _, stmt := range fn.Body.List {
			dispatch, ok := stmt.(*ast.SwitchStmt)
			if !ok {
				continue
			}
			selector, ok := dispatch.Tag.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Type" {
				continue
			}
			command, ok := selector.X.(*ast.Ident)
			if !ok || command.Name != "cmd" {
				continue
			}
			commands := make(map[string]bool)
			for _, stmt := range dispatch.Body.List {
				for _, expr := range stmt.(*ast.CaseClause).List {
					literal, ok := expr.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						t.Fatalf("unexpected user command case: %T", expr)
					}
					name, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatal(err)
					}
					commands[name] = true
				}
			}
			return commands
		}
	}
	t.Fatal("ExecuteUserCommand cmd.Type switch not found")
	return nil
}
