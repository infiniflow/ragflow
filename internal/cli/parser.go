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
	original  string
}

// NewParser creates a new parser
func NewParser(input string) *Parser {
	l := NewLexer(input)
	p := &Parser{lexer: l, original: input}
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
func (p *Parser) Parse(cliMode CommandLineMode) (*Command, error) {
	if p.curToken.Type == TokenEOF {
		return nil, nil
	}

	// Check for meta commands (backslash commands)
	if p.curToken.Type == TokenIdentifier && strings.HasPrefix(p.curToken.Value, "\\") {
		return p.parseMetaCommand()
	}

	return p.parseCommand(cliMode)
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
	case TokenCheck:
		return p.parseAdminCheckCommand()
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
	case TokenRetrieve:
		return p.parseAdminRetrieveCommand()
	case TokenParse:
		return p.parseAdminParseCommand()
	case TokenBenchmark:
		return p.parseAdminBenchmarkCommand()

	case TokenStartup:
		return p.parseAdminStartupCommand()
	case TokenShutdown:
		return p.parseAdminShutdownCommand()
	case TokenRestart:
		return p.parseAdminRestartCommand()
	case TokenMQ:
		return p.parseMessageQueueCommand()
	case TokenRemove:
		return p.parseAdminRemoveCommand()
	case TokenStop:
		return p.parseAdminStopIngestionTasks()
	case TokenAdd:
		return p.parseAdminAddCommand()
	case TokenDelete:
		return p.parseAdminDeleteCommand()
	case TokenSave:
		return p.parseAdminSaveCommand()
	case TokenUse:
		return p.parseAdminUseCommand()
	case TokenPurge:
		return p.parseAdminPurgeCommand()
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
	case TokenRetrieve:
		return p.parseRetrieveCommand()
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
	case TokenOpenaiChat:
		return p.parseOpenaiChatCommand()
	case TokenThink:
		return p.parseThinkCommand()
	case TokenEmbed:
		return p.parseEmbedCommand()
	case TokenRerank:
		return p.parseRerankCommand()
	case TokenASR:
		return p.parseASRCommand()
	case TokenTTS:
		return p.parseTTSCommand()
	case TokenOCR:
		return p.parseOCRCommand()
	case TokenCheck:
		return p.parseCheckCommand()
	case TokenStart:
		return p.parseUserStartIngestion()
	case TokenStop:
		return p.parseUserStopIngestion()

	case TokenSave:
		return p.parseUserSaveCommand()
	case TokenUse:
		return p.parseUseCommand()
	case TokenUpdate:
		return p.parseUpdateCommand()
	case TokenRemove:
		return p.parseRemoveCommand()
	case TokenGet:
		return p.parseGetCommand()
	case TokenExplain:
		return p.parseExplainCommand()
	case TokenChunk:
		return p.parseChunkCommand(false)

	case TokenLS, TokenCat, TokenSearch:
		// For context engine
		return p.parseFileSystemCommand()
	default:
		return nil, fmt.Errorf("unknown command: %s", p.curToken.Value)
	}
}

func (p *Parser) parseCommand(cliMode CommandLineMode) (*Command, error) {
	if p.curToken.Type != TokenIdentifier && !isKeyword(p.curToken.Type) {
		return nil, fmt.Errorf("expected command, got %s", p.curToken.Value)
	}

	switch cliMode {
	case AdminMode:
		return p.parseAdminCommand()
	case APIMode:
		return p.parseUserCommand()
	default:
		return nil, fmt.Errorf("unknown mode: %s", cliMode)
	}
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
	return tokenType >= TokenLogin && tokenType <= TokenPanic
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

func (p *Parser) parseVariableValue() (string, error) {
	switch p.curToken.Type {
	case TokenIdentifier, TokenQuotedString, TokenInteger, TokenFloat:
		return p.curToken.Value, nil
	default:
		return "", fmt.Errorf("expected variable value, got %s", p.curToken.Value)
	}
}

func (p *Parser) parseNumber() (int, error) {
	if p.curToken.Type != TokenInteger {
		return 0, fmt.Errorf("expected number, got %s", p.curToken.Value)
	}
	return strconv.Atoi(p.curToken.Value)
}

func (p *Parser) parseFloat() (float64, error) {
	// Accept either TokenInteger or TokenFloat so that literals like
	// `0.3` (which the lexer tags as TokenFloat) and `10` (TokenInteger)
	// both parse cleanly.
	if p.curToken.Type != TokenInteger && p.curToken.Type != TokenFloat {
		return math.NaN(), fmt.Errorf("expected number, got %s", p.curToken.Value)
	}
	result, err := strconv.ParseFloat(p.curToken.Value, 64)
	if err != nil {
		return math.NaN(), err
	}

	return result, nil
}

// parseQuotedStringList consumes a bracket-delimited list of quoted strings:
//
//	[ 'a', 'b', 'c' ]
//
// Empty list [] is allowed. The cursor must be positioned on '[' when called;
// on return, the cursor is positioned just past the closing ']'.
func (p *Parser) parseQuotedStringList() ([]string, error) {
	if p.curToken.Type != TokenLBracket {
		return nil, fmt.Errorf("expected '[', got %s", p.curToken.Value)
	}
	p.nextToken() // skip '['

	// Always return a non-nil slice so callers (and json.Marshal) see []
	// instead of null for the empty-list case.
	list := make([]string, 0)
	// Allow empty list []
	if p.curToken.Type == TokenRBracket {
		p.nextToken() // skip ']'
		return list, nil
	}

	for {
		s, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected quoted string in list: %w", err)
		}
		list = append(list, s)
		p.nextToken() // step past the closing quote

		if p.curToken.Type == TokenComma {
			p.nextToken() // step past ','
			continue
		}
		if p.curToken.Type == TokenRBracket {
			p.nextToken() // step past ']'
			return list, nil
		}
		return nil, fmt.Errorf("expected ',' or ']' in list, got %s", p.curToken.Value)
	}
}

func tokenTypeToString(t int) string {
	switch t {
	case TokenEOF:
		return "end of input"
	case TokenIdentifier:
		return "identifier"
	case TokenInteger:
		return "integer"
	case TokenFloat:
		return "float"
	case TokenQuotedString:
		return "quoted string"
	case TokenLBracket:
		return "'['"
	case TokenRBracket:
		return "']'"
	case TokenComma:
		return "','"
	case TokenSemicolon:
		return "';'"
	}
	return fmt.Sprintf("token(%d)", t)
}

func (p *Parser) parseFileSystemCommand() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("file_system_command")
	cmd.Params["command"] = p.original

	return cmd, nil
}
