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
	"math"
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
	// Note: These are now handled in parseUserCommand to support both SQL-style and CE-style syntax
	// if p.curToken.Type == TokenIdentifier && isCECommand(p.curToken.Value) {
	// 	return p.parseCECommand()
	// }

	return p.parseCommand(adminCommand)
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
	case TokenInsert:
		return p.parseInsertCommand()
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
	case TokenEnable:
		return p.parseEnableCommand()
	case TokenDisable:
		return p.parseDisableCommand()
	case TokenStream:
		return p.parseStreamCommand()
	case TokenChat:
		return p.parseChatCommand()
	case TokenThink:
		return p.parseThinkCommand()
	case TokenLS:
		return p.parseCEListCommand()
	case TokenCat:
		return p.parseCECatCommand()
	case TokenUse:
		return p.parseUseCommand()
	case TokenUpdate:
		return p.parseUpdateCommand()
	case TokenRemove:
		return p.parseRemoveCommand()
	case TokenMount:
		return p.parseContextMountCommand()
	case TokenUnmount:
		return p.parseContextUnmountCommand()
	default:
		return nil, fmt.Errorf("unknown command: %s", p.curToken.Value)
	}
}

func (p *Parser) parseCommand(adminCommand bool) (*Command, error) {
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
	return tokenType >= TokenLogin && tokenType <= TokenTag
}

// isCECommand checks if the given string is a Filesystem command
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
	if p.curToken.Type != TokenInteger {
		return 0, fmt.Errorf("expected number, got %s", p.curToken.Value)
	}
	return strconv.Atoi(p.curToken.Value)
}

func (p *Parser) parseFloat() (float64, error) {
	if p.curToken.Type != TokenInteger {
		return math.NaN(), fmt.Errorf("expected number, got %s", p.curToken.Value)
	}
	result, err := strconv.ParseFloat(p.curToken.Value, 64)
	if err != nil {
		return math.NaN(), err
	}

	return result, nil
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
	case "CAT":
		return p.parseCECatCommand()
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
		p.nextToken()

		// Handle path components separated by slashes (e.g., "skills/hub1")
		for p.curToken.Type == TokenSlash {
			p.nextToken() // consume slash
			if p.curToken.Type == TokenIdentifier || p.curToken.Type == TokenDatasets ||
				p.curToken.Type == TokenAgents || p.curToken.Type == TokenChats {
				path = path + "/" + p.curToken.Value
				p.nextToken()
			} else if p.curToken.Type == TokenNumber {
				// Handle version numbers like 1.0.0 (parsed as number . number . number)
				// OR filenames starting with numbers like 3_list_compressors.pdf
				numberPart := p.curToken.Value
				p.nextToken()
				// Continue reading .number parts (version number format)
				if p.curToken.Type == TokenIllegal && p.curToken.Value == "." {
					versionPart := numberPart
					for p.curToken.Type == TokenIllegal && p.curToken.Value == "." {
						p.nextToken() // consume .
						if p.curToken.Type == TokenNumber {
							versionPart = versionPart + "." + p.curToken.Value
							p.nextToken()
						} else {
							break
						}
					}
					path = path + "/" + versionPart
				} else if p.curToken.Type == TokenIdentifier {
					// Filename starting with number: 3_list_compressors.pdf
					path = path + "/" + numberPart + p.curToken.Value
					p.nextToken()
				} else {
					// Just a number
					path = path + "/" + numberPart
				}
			} else {
				// Trailing slash, just append it
				path = path + "/"
				break
			}
		}

		cmd.Params["path"] = path
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

// parseCECatCommand parses the cat command
// Syntax: cat <path>
func (p *Parser) parseCECatCommand() (*Command, error) {
	p.nextToken() // consume CAT

	cmd := NewCommand("ce_cat")

	if p.curToken.Type != TokenIdentifier && p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected path after CAT")
	}

	path := p.curToken.Value
	if p.curToken.Type == TokenQuotedString {
		path = strings.Trim(path, "\"'")
	}
	p.nextToken()

	// Handle path components separated by slashes (e.g., "skills/hub1/skill/README.md")
	for p.curToken.Type == TokenSlash {
		p.nextToken() // consume slash
		if p.curToken.Type == TokenIdentifier || p.curToken.Type == TokenAgents ||
			p.curToken.Type == TokenChats || p.curToken.Type == TokenDatasets {
			path = path + "/" + p.curToken.Value
			p.nextToken()
		} else if p.curToken.Type == TokenNumber {
			// Handle version numbers like 1.0.0 (parsed as number . number . number)
			// OR filenames starting with numbers like 3_list_compressors.pdf
			numberPart := p.curToken.Value
			p.nextToken()
			// Continue reading .number parts (version number format)
			if p.curToken.Type == TokenIllegal && p.curToken.Value == "." {
				versionPart := numberPart
				for p.curToken.Type == TokenIllegal && p.curToken.Value == "." {
					p.nextToken() // consume .
					if p.curToken.Type == TokenNumber {
						versionPart = versionPart + "." + p.curToken.Value
						p.nextToken()
					} else {
						break
					}
				}
				path = path + "/" + versionPart
			} else if p.curToken.Type == TokenIdentifier {
				// Filename starting with number: 3_list_compressors.pdf
				path = path + "/" + numberPart + p.curToken.Value
				p.nextToken()
			} else {
				// Just a number
				path = path + "/" + numberPart
			}
		} else if p.curToken.Type == TokenQuotedString {
			path = path + "/" + strings.Trim(p.curToken.Value, "\"'")
			p.nextToken()
		} else {
			// Trailing slash, just append it
			path = path + "/"
			break
		}
	}

	cmd.Params["path"] = path

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
		p.nextToken()

		// Handle path components separated by slashes (e.g., "skills/hub1")
		for p.curToken.Type == TokenSlash {
			p.nextToken() // consume slash
			if p.curToken.Type == TokenIdentifier || p.curToken.Type == TokenAgents ||
				p.curToken.Type == TokenChats || p.curToken.Type == TokenDatasets {
				path = path + "/" + p.curToken.Value
				p.nextToken()
		} else if p.curToken.Type == TokenNumber {
			// Handle version numbers like 1.0.0 (parsed as number . number . number)
			// OR filenames starting with numbers like 3_list_compressors.pdf
			numberPart := p.curToken.Value
			p.nextToken()
			// Continue reading .number parts (version number format)
			if p.curToken.Type == TokenIllegal && p.curToken.Value == "." {
				versionPart := numberPart
				for p.curToken.Type == TokenIllegal && p.curToken.Value == "." {
					p.nextToken() // consume .
					if p.curToken.Type == TokenNumber {
						versionPart = versionPart + "." + p.curToken.Value
						p.nextToken()
					} else {
						break
					}
				}
				path = path + "/" + versionPart
			} else if p.curToken.Type == TokenIdentifier {
				// Filename starting with number: 3_list_compressors.pdf
				path = path + "/" + numberPart + p.curToken.Value
				p.nextToken()
			} else {
				// Just a number
				path = path + "/" + numberPart
			}
		} else if p.curToken.Type == TokenQuotedString {
			path = path + "/" + strings.Trim(p.curToken.Value, "\"'")
			p.nextToken()
		} else {
			// Trailing slash, just append it
			path = path + "/"
			break
		}
	}

	cmd.Params["path"] = path
	} else {
		cmd.Params["path"] = "."
	}

	// Optional semicolon
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}
