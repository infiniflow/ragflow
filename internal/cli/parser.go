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
	"fmt"
	"strconv"
	"strings"
)

// Parser implements a recursive descent parser for RAGFlow CLI commands
type Parser struct {
	lexer     *Lexer
	curToken  Token
	peekToken Token
}

// NewParser creates a new parser
func NewParser(input string) *Parser {
	l := NewLexer(input)
	p := &Parser{lexer: l}
	// Read two tokens to initialize curToken and peekToken
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

// Parse parses the input and returns a Command
func (p *Parser) Parse(adminCommand bool) (*Command, error) {
	if p.curToken.Type == TokenEOF {
		return nil, nil
	}

	// Check for meta commands (backslash commands)
	if p.curToken.Type == TokenIdentifier && strings.HasPrefix(p.curToken.Value, "\\") {
		return p.parseMetaCommand()
	}

	// Check for ContextEngine commands (ls, cat, search)
	if p.curToken.Type == TokenIdentifier && isCECommand(p.curToken.Value) {
		return p.parseCECommand()
	}

	// Parse SQL-like command
	return p.parseSQLCommand(adminCommand)
}

func (p *Parser) parseMetaCommand() (*Command, error) {
	cmd := NewCommand("meta")
	cmdName := strings.TrimPrefix(p.curToken.Value, "\\")
	cmd.Params["command"] = strings.ToLower(cmdName)

	// Parse arguments
	var args []string
	p.nextToken()
	for p.curToken.Type != TokenEOF {
		args = append(args, p.curToken.Value)
		p.nextToken()
	}
	cmd.Params["args"] = args

	return cmd, nil
}

func (p *Parser) parseAdminCommand() (*Command, error) {

	switch p.curToken.Type {
	case TokenLogin:
		return p.parseAdminLoginUser()
	case TokenLogout:
		return p.parseAdminLogout()
	case TokenPing:
		return p.parseAdminPingServer()
	case TokenList:
		return p.parseAdminListCommand()
	case TokenShow:
		return p.parseAdminShowCommand()
	case TokenCreate:
		return p.parseAdminCreateCommand()
	case TokenDrop:
		return p.parseAdminDropCommand()
	case TokenAlter:
		return p.parseAdminAlterCommand()
	case TokenGrant:
		return p.parseAdminGrantCommand()
	case TokenRevoke:
		return p.parseAdminRevokeCommand()
	case TokenSet:
		return p.parseAdminSetCommand()
	case TokenUnset:
		return p.parseAdminUnsetCommand()
	case TokenReset:
		return p.parseAdminResetCommand()
	case TokenGenerate:
		return p.parseAdminGenerateCommand()
	case TokenImport:
		return p.parseAdminImportCommand()
	case TokenSearch:
		return p.parseAdminSearchCommand()
	case TokenParse:
		return p.parseAdminParseCommand()
	case TokenBenchmark:
		return p.parseAdminBenchmarkCommand()
	case TokenRegister:
		return p.parseAdminRegisterCommand()
	case TokenStartup:
		return p.parseAdminStartupCommand()
	case TokenShutdown:
		return p.parseAdminShutdownCommand()
	case TokenRestart:
		return p.parseAdminRestartCommand()
	default:
		return nil, fmt.Errorf("unknown command: %s", p.curToken.Value)
	}
}

func (p *Parser) parseUserCommand() (*Command, error) {

	switch p.curToken.Type {
	case TokenLogin:
		return p.parseLoginUser()
	case TokenLogout:
		return p.parseLogout()
	case TokenPing:
		return p.parsePingServer()
	case TokenList:
		return p.parseListCommand()
	case TokenShow:
		return p.parseShowCommand()
	case TokenCreate:
		return p.parseCreateCommand()
	case TokenDrop:
		return p.parseDropCommand()
	case TokenAdd:
		return p.parseAddCommand()
	case TokenDelete:
		return p.parseDeleteCommand()
	case TokenAlter:
		return p.parseAlterCommand()
	case TokenGrant:
		return p.parseGrantCommand()
	case TokenRevoke:
		return p.parseRevokeCommand()
	case TokenSet:
		return p.parseSetCommand()
	case TokenUnset:
		return p.parseUnsetCommand()
	case TokenReset:
		return p.parseResetCommand()
	case TokenGenerate:
		return p.parseGenerateCommand()
	case TokenImport:
		return p.parseImportCommand()
	case TokenSearch:
		return p.parseSearchCommand()
	case TokenParse:
		return p.parseParseCommand()
	case TokenBenchmark:
		return p.parseBenchmarkCommand()
	case TokenRegister:
		return p.parseRegisterCommand()
	case TokenStartup:
		return p.parseStartupCommand()
	case TokenShutdown:
		return p.parseShutdownCommand()
	case TokenRestart:
		return p.parseRestartCommand()
	default:
		return nil, fmt.Errorf("unknown command: %s", p.curToken.Value)
	}
}

func (p *Parser) parseSQLCommand(adminCommand bool) (*Command, error) {
	if p.curToken.Type != TokenIdentifier && !isKeyword(p.curToken.Type) {
		return nil, fmt.Errorf("expected command, got %s", p.curToken.Value)
	}

	if adminCommand {
		return p.parseAdminCommand()
	}

	return p.parseUserCommand()
}

func (p *Parser) expectPeek(tokenType int) error {
	if p.peekToken.Type != tokenType {
		return fmt.Errorf("expected %s, got %s", tokenTypeToString(tokenType), p.peekToken.Value)
	}
	p.nextToken()
	return nil
}

func (p *Parser) expectSemicolon() error {
	if p.curToken.Type == TokenSemicolon {
		return nil
	}
	if p.peekToken.Type == TokenSemicolon {
		p.nextToken()
		return nil
	}
	return fmt.Errorf("expected semicolon")
}

func isKeyword(tokenType int) bool {
	return tokenType >= TokenLogin && tokenType <= TokenDocMeta
}

// isCECommand checks if the given string is a ContextEngine command
func isCECommand(s string) bool {
	upper := strings.ToUpper(s)
	switch upper {
	case "LS", "LIST", "SEARCH":
		return true
	}
	return false
}

// Helper functions for parsing
func (p *Parser) parseQuotedString() (string, error) {
	if p.curToken.Type != TokenQuotedString {
		return "", fmt.Errorf("expected quoted string, got %s", p.curToken.Value)
	}
	return p.curToken.Value, nil
}

func (p *Parser) parseIdentifier() (string, error) {
	if p.curToken.Type != TokenIdentifier {
		return "", fmt.Errorf("expected identifier, got %s", p.curToken.Value)
	}
	return p.curToken.Value, nil
}

func (p *Parser) parseNumber() (int, error) {
	if p.curToken.Type != TokenNumber {
		return 0, fmt.Errorf("expected number, got %s", p.curToken.Value)
	}
	return strconv.Atoi(p.curToken.Value)
}

func tokenTypeToString(t int) string {
	// Simplified for error messages
	return fmt.Sprintf("token(%d)", t)
}

// parseCECommand parses ContextEngine commands (ls, search)
func (p *Parser) parseCECommand() (*Command, error) {
	cmdName := strings.ToUpper(p.curToken.Value)

	switch cmdName {
	case "LS", "LIST":
		return p.parseCEListCommand()
	case "SEARCH":
		return p.parseCESearchCommand()
	default:
		return nil, fmt.Errorf("unknown ContextEngine command: %s", cmdName)
	}
}

// parseCEListCommand parses the ls command
// Syntax: ls [path] or ls datasets
func (p *Parser) parseCEListCommand() (*Command, error) {
	p.nextToken() // consume LS/LIST

	cmd := NewCommand("ce_ls")

	// Check if there's a path argument
	// Also accept TokenDatasets since "datasets" is a keyword but can be a path
	if p.curToken.Type == TokenIdentifier || p.curToken.Type == TokenQuotedString ||
		p.curToken.Type == TokenDatasets {
		path := p.curToken.Value
		// Remove quotes if present
		if p.curToken.Type == TokenQuotedString {
			path = strings.Trim(path, "\"'")
		}
		cmd.Params["path"] = path
		p.nextToken()
	} else {
		// Default to "datasets" root
		cmd.Params["path"] = "datasets"
	}

	// Optional semicolon
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseCESearchCommand parses the search command
// Syntax: search <query> or search <query> in <path>
func (p *Parser) parseCESearchCommand() (*Command, error) {
	p.nextToken() // consume SEARCH

	cmd := NewCommand("ce_search")

	if p.curToken.Type != TokenIdentifier && p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected query after SEARCH")
	}

	query := p.curToken.Value
	if p.curToken.Type == TokenQuotedString {
		query = strings.Trim(query, "\"'")
	}
	cmd.Params["query"] = query
	p.nextToken()

	// Check for optional "in <path>" clause
	if p.curToken.Type == TokenIdentifier && strings.ToUpper(p.curToken.Value) == "IN" {
		p.nextToken() // consume IN

		if p.curToken.Type != TokenIdentifier && p.curToken.Type != TokenQuotedString {
			return nil, fmt.Errorf("expected path after IN")
		}

		path := p.curToken.Value
		if p.curToken.Type == TokenQuotedString {
			path = strings.Trim(path, "\"'")
		}
		cmd.Params["path"] = path
		p.nextToken()
	} else {
		cmd.Params["path"] = "."
	}

	// Optional semicolon
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}
