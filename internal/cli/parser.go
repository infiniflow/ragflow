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
func (p *Parser) Parse() (*Command, error) {
	if p.curToken.Type == TokenEOF {
		return nil, nil
	}

	// Check for meta commands (backslash commands)
	if p.curToken.Type == TokenIdentifier && strings.HasPrefix(p.curToken.Value, "\\") {
		return p.parseMetaCommand()
	}

	// Parse SQL-like command
	return p.parseSQLCommand()
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

func (p *Parser) parseSQLCommand() (*Command, error) {
	if p.curToken.Type != TokenIdentifier && !isKeyword(p.curToken.Type) {
		return nil, fmt.Errorf("expected command, got %s", p.curToken.Value)
	}

	switch p.curToken.Type {
	case TokenLogin:
		return p.parseLoginUser()
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
	case TokenAlter:
		return p.parseAlterCommand()
	case TokenGrant:
		return p.parseGrantCommand()
	case TokenRevoke:
		return p.parseRevokeCommand()
	case TokenSet:
		return p.parseSetCommand()
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
	return tokenType >= TokenLogin && tokenType <= TokenPing
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

// Command parsers
func (p *Parser) parseLoginUser() (*Command, error) {
	cmd := NewCommand("login_user")

	p.nextToken() // consume LOGIN
	if p.curToken.Type != TokenUser {
		return nil, fmt.Errorf("expected USER after LOGIN")
	}

	p.nextToken()
	email, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd.Params["email"] = email

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (p *Parser) parsePingServer() (*Command, error) {
	cmd := NewCommand("ping_server")
	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseRegisterCommand() (*Command, error) {
	cmd := NewCommand("register_user")

	p.nextToken() // consume REGISTER
	if err := p.expectPeek(TokenUser); err != nil {
		return nil, err
	}
	p.nextToken()

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd.Params["user_name"] = userName

	p.nextToken()
	if p.curToken.Type != TokenAs {
		return nil, fmt.Errorf("expected AS")
	}

	p.nextToken()
	nickname, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd.Params["nickname"] = nickname

	p.nextToken()
	if p.curToken.Type != TokenPassword {
		return nil, fmt.Errorf("expected PASSWORD")
	}

	p.nextToken()
	password, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd.Params["password"] = password

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (p *Parser) parseListCommand() (*Command, error) {
	p.nextToken() // consume LIST

	switch p.curToken.Type {
	case TokenServices:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("list_services"), nil
	case TokenUsers:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("list_users"), nil
	case TokenRoles:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("list_roles"), nil
	case TokenVars:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("list_variables"), nil
	case TokenConfigs:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("list_configs"), nil
	case TokenEnvs:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("list_environments"), nil
	case TokenDatasets:
		return p.parseListDatasets()
	case TokenAgents:
		return p.parseListAgents()
	case TokenKeys:
		return p.parseListKeys()
	case TokenModel:
		return p.parseListModelProviders()
	case TokenDefault:
		return p.parseListDefaultModels()
	case TokenChats:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("list_user_chats"), nil
	case TokenFiles:
		return p.parseListFiles()
	default:
		return nil, fmt.Errorf("unknown LIST target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseListDatasets() (*Command, error) {
	cmd := NewCommand("list_user_datasets")
	p.nextToken() // consume DATASETS

	if p.curToken.Type == TokenSemicolon {
		return cmd, nil
	}

	if p.curToken.Type == TokenOf {
		p.nextToken()
		userName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd = NewCommand("list_datasets")
		cmd.Params["user_name"] = userName
		p.nextToken()
	}

	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseListAgents() (*Command, error) {
	p.nextToken() // consume AGENTS

	if p.curToken.Type == TokenSemicolon {
		return NewCommand("list_user_agents"), nil
	}

	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF")
	}
	p.nextToken()

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("list_agents")
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseListKeys() (*Command, error) {
	p.nextToken() // consume KEYS
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF")
	}
	p.nextToken()

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("list_keys")
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseListModelProviders() (*Command, error) {
	p.nextToken() // consume MODEL
	if p.curToken.Type != TokenProviders {
		return nil, fmt.Errorf("expected PROVIDERS")
	}
	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return NewCommand("list_user_model_providers"), nil
}

func (p *Parser) parseListDefaultModels() (*Command, error) {
	p.nextToken() // consume DEFAULT
	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS")
	}
	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return NewCommand("list_user_default_models"), nil
}

func (p *Parser) parseListFiles() (*Command, error) {
	p.nextToken() // consume FILES
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF")
	}
	p.nextToken()
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET")
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("list_user_dataset_files")
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseShowCommand() (*Command, error) {
	p.nextToken() // consume SHOW

	switch p.curToken.Type {
	case TokenVersion:
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("show_version"), nil
	case TokenCurrent:
		p.nextToken()
		if p.curToken.Type != TokenUser {
			return nil, fmt.Errorf("expected USER after CURRENT")
		}
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return NewCommand("show_current_user"), nil
	case TokenUser:
		return p.parseShowUser()
	case TokenRole:
		return p.parseShowRole()
	case TokenVar:
		return p.parseShowVariable()
	case TokenService:
		return p.parseShowService()
	default:
		return nil, fmt.Errorf("unknown SHOW target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseShowUser() (*Command, error) {
	p.nextToken() // consume USER

	// Check for PERMISSION
	if p.curToken.Type == TokenPermission {
		p.nextToken()
		userName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd := NewCommand("show_user_permission")
		cmd.Params["user_name"] = userName
		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return cmd, nil
	}

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("show_user")
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseShowRole() (*Command, error) {
	p.nextToken() // consume ROLE
	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("show_role")
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseShowVariable() (*Command, error) {
	p.nextToken() // consume VAR
	varName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("show_variable")
	cmd.Params["var_name"] = varName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseShowService() (*Command, error) {
	p.nextToken() // consume SERVICE
	serviceNum, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("show_service")
	cmd.Params["number"] = serviceNum

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseCreateCommand() (*Command, error) {
	p.nextToken() // consume CREATE

	switch p.curToken.Type {
	case TokenUser:
		return p.parseCreateUser()
	case TokenRole:
		return p.parseCreateRole()
	case TokenModel:
		return p.parseCreateModelProvider()
	case TokenDataset:
		return p.parseCreateDataset()
	case TokenChat:
		return p.parseCreateChat()
	default:
		return nil, fmt.Errorf("unknown CREATE target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseCreateUser() (*Command, error) {
	p.nextToken() // consume USER
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	password, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("create_user")
	cmd.Params["user_name"] = userName
	cmd.Params["password"] = password
	cmd.Params["role"] = "user"

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseCreateRole() (*Command, error) {
	p.nextToken() // consume ROLE
	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("create_role")
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if p.curToken.Type == TokenDescription {
		p.nextToken()
		description, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["description"] = description
		p.nextToken()
	}

	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseCreateModelProvider() (*Command, error) {
	p.nextToken() // consume MODEL
	if p.curToken.Type != TokenProvider {
		return nil, fmt.Errorf("expected PROVIDER")
	}
	p.nextToken()

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	providerKey, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("create_model_provider")
	cmd.Params["provider_name"] = providerName
	cmd.Params["provider_key"] = providerKey

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseCreateDataset() (*Command, error) {
	p.nextToken() // consume DATASET
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH")
	}
	p.nextToken()
	if p.curToken.Type != TokenEmbedding {
		return nil, fmt.Errorf("expected EMBEDDING")
	}
	p.nextToken()

	embedding, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	cmd := NewCommand("create_user_dataset")
	cmd.Params["dataset_name"] = datasetName
	cmd.Params["embedding"] = embedding

	if p.curToken.Type == TokenParser {
		p.nextToken()
		parserType, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["parser_type"] = parserType
		p.nextToken()
	} else if p.curToken.Type == TokenPipeline {
		p.nextToken()
		pipeline, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["pipeline"] = pipeline
		p.nextToken()
	} else {
		return nil, fmt.Errorf("expected PARSER or PIPELINE")
	}

	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseCreateChat() (*Command, error) {
	p.nextToken() // consume CHAT
	chatName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("create_user_chat")
	cmd.Params["chat_name"] = chatName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseDropCommand() (*Command, error) {
	p.nextToken() // consume DROP

	switch p.curToken.Type {
	case TokenUser:
		return p.parseDropUser()
	case TokenRole:
		return p.parseDropRole()
	case TokenModel:
		return p.parseDropModelProvider()
	case TokenDataset:
		return p.parseDropDataset()
	case TokenChat:
		return p.parseDropChat()
	case TokenKey:
		return p.parseDropKey()
	default:
		return nil, fmt.Errorf("unknown DROP target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseDropUser() (*Command, error) {
	p.nextToken() // consume USER
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_user")
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseDropRole() (*Command, error) {
	p.nextToken() // consume ROLE
	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_role")
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseDropModelProvider() (*Command, error) {
	p.nextToken() // consume MODEL
	if p.curToken.Type != TokenProvider {
		return nil, fmt.Errorf("expected PROVIDER")
	}
	p.nextToken()

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_model_provider")
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseDropDataset() (*Command, error) {
	p.nextToken() // consume DATASET
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_user_dataset")
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseDropChat() (*Command, error) {
	p.nextToken() // consume CHAT
	chatName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_user_chat")
	cmd.Params["chat_name"] = chatName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseDropKey() (*Command, error) {
	p.nextToken() // consume KEY
	key, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF")
	}
	p.nextToken()

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_key")
	cmd.Params["key"] = key
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseAlterCommand() (*Command, error) {
	p.nextToken() // consume ALTER

	switch p.curToken.Type {
	case TokenUser:
		return p.parseAlterUser()
	case TokenRole:
		return p.parseAlterRole()
	default:
		return nil, fmt.Errorf("unknown ALTER target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAlterUser() (*Command, error) {
	p.nextToken() // consume USER

	if p.curToken.Type == TokenActive {
		return p.parseActivateUser()
	}

	if p.curToken.Type == TokenPassword {
		p.nextToken()
		userName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}

		p.nextToken()
		password, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}

		cmd := NewCommand("alter_user")
		cmd.Params["user_name"] = userName
		cmd.Params["password"] = password

		p.nextToken()
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return cmd, nil
	}

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenSet {
		return nil, fmt.Errorf("expected SET")
	}
	p.nextToken()
	if p.curToken.Type != TokenRole {
		return nil, fmt.Errorf("expected ROLE")
	}
	p.nextToken()

	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("alter_user_role")
	cmd.Params["user_name"] = userName
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseActivateUser() (*Command, error) {
	p.nextToken() // consume ACTIVE
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	status, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("activate_user")
	cmd.Params["user_name"] = userName
	cmd.Params["activate_status"] = status

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseAlterRole() (*Command, error) {
	p.nextToken() // consume ROLE
	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenSet {
		return nil, fmt.Errorf("expected SET")
	}
	p.nextToken()
	if p.curToken.Type != TokenDescription {
		return nil, fmt.Errorf("expected DESCRIPTION")
	}
	p.nextToken()

	description, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("alter_role")
	cmd.Params["role_name"] = roleName
	cmd.Params["description"] = description

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseGrantCommand() (*Command, error) {
	p.nextToken() // consume GRANT

	if p.curToken.Type == TokenAdmin {
		return p.parseGrantAdmin()
	}

	return p.parseGrantPermission()
}

func (p *Parser) parseGrantAdmin() (*Command, error) {
	p.nextToken() // consume ADMIN
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("grant_admin")
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseGrantPermission() (*Command, error) {
	actions, err := p.parseIdentifierList()
	if err != nil {
		return nil, err
	}

	if p.curToken.Type != TokenOn {
		return nil, fmt.Errorf("expected ON")
	}
	p.nextToken()

	resource, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenTo {
		return nil, fmt.Errorf("expected TO")
	}
	p.nextToken()
	if p.curToken.Type != TokenRole {
		return nil, fmt.Errorf("expected ROLE")
	}
	p.nextToken()

	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("grant_permission")
	cmd.Params["actions"] = actions
	cmd.Params["resource"] = resource
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseRevokeCommand() (*Command, error) {
	p.nextToken() // consume REVOKE

	if p.curToken.Type == TokenAdmin {
		return p.parseRevokeAdmin()
	}

	return p.parseRevokePermission()
}

func (p *Parser) parseRevokeAdmin() (*Command, error) {
	p.nextToken() // consume ADMIN
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("revoke_admin")
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseRevokePermission() (*Command, error) {
	actions, err := p.parseIdentifierList()
	if err != nil {
		return nil, err
	}

	if p.curToken.Type != TokenOn {
		return nil, fmt.Errorf("expected ON")
	}
	p.nextToken()

	resource, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()
	if p.curToken.Type != TokenRole {
		return nil, fmt.Errorf("expected ROLE")
	}
	p.nextToken()

	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("revoke_permission")
	cmd.Params["actions"] = actions
	cmd.Params["resource"] = resource
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseIdentifierList() ([]string, error) {
	var list []string

	ident, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}
	list = append(list, ident)
	p.nextToken()

	for p.curToken.Type == TokenComma {
		p.nextToken()
		ident, err := p.parseIdentifier()
		if err != nil {
			return nil, err
		}
		list = append(list, ident)
		p.nextToken()
	}

	return list, nil
}

func (p *Parser) parseSetCommand() (*Command, error) {
	p.nextToken() // consume SET

	if p.curToken.Type == TokenVar {
		return p.parseSetVariable()
	}
	if p.curToken.Type == TokenDefault {
		return p.parseSetDefault()
	}

	return nil, fmt.Errorf("unknown SET target: %s", p.curToken.Value)
}

func (p *Parser) parseSetVariable() (*Command, error) {
	p.nextToken() // consume VAR
	varName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	varValue, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("set_variable")
	cmd.Params["var_name"] = varName
	cmd.Params["var_value"] = varValue

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseSetDefault() (*Command, error) {
	p.nextToken() // consume DEFAULT

	var modelType, modelID string

	switch p.curToken.Type {
	case TokenLLM:
		modelType = "llm_id"
	case TokenVLM:
		modelType = "img2txt_id"
	case TokenEmbedding:
		modelType = "embd_id"
	case TokenReranker:
		modelType = "reranker_id"
	case TokenASR:
		modelType = "asr_id"
	case TokenTTS:
		modelType = "tts_id"
	default:
		return nil, fmt.Errorf("unknown model type: %s", p.curToken.Value)
	}

	p.nextToken()
	id, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	modelID = id

	cmd := NewCommand("set_default_model")
	cmd.Params["model_type"] = modelType
	cmd.Params["model_id"] = modelID

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseResetCommand() (*Command, error) {
	p.nextToken() // consume RESET

	if p.curToken.Type != TokenDefault {
		return nil, fmt.Errorf("expected DEFAULT")
	}
	p.nextToken()

	var modelType string
	switch p.curToken.Type {
	case TokenLLM:
		modelType = "llm_id"
	case TokenVLM:
		modelType = "img2txt_id"
	case TokenEmbedding:
		modelType = "embd_id"
	case TokenReranker:
		modelType = "reranker_id"
	case TokenASR:
		modelType = "asr_id"
	case TokenTTS:
		modelType = "tts_id"
	default:
		return nil, fmt.Errorf("unknown model type: %s", p.curToken.Value)
	}

	cmd := NewCommand("reset_default_model")
	cmd.Params["model_type"] = modelType

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseGenerateCommand() (*Command, error) {
	p.nextToken() // consume GENERATE
	if p.curToken.Type != TokenKey {
		return nil, fmt.Errorf("expected KEY")
	}
	p.nextToken()
	if p.curToken.Type != TokenFor {
		return nil, fmt.Errorf("expected FOR")
	}
	p.nextToken()
	if p.curToken.Type != TokenUser {
		return nil, fmt.Errorf("expected USER")
	}
	p.nextToken()

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("generate_key")
	cmd.Params["user_name"] = userName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseImportCommand() (*Command, error) {
	p.nextToken() // consume IMPORT
	documentPaths, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenInto {
		return nil, fmt.Errorf("expected INTO")
	}
	p.nextToken()
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET")
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("import_docs_into_dataset")
	cmd.Params["document_paths"] = documentPaths
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseSearchCommand() (*Command, error) {
	p.nextToken() // consume SEARCH
	question, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenOn {
		return nil, fmt.Errorf("expected ON")
	}
	p.nextToken()
	if p.curToken.Type != TokenDatasets {
		return nil, fmt.Errorf("expected DATASETS")
	}
	p.nextToken()

	datasets, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("search_on_datasets")
	cmd.Params["question"] = question
	cmd.Params["datasets"] = datasets

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseParseCommand() (*Command, error) {
	p.nextToken() // consume PARSE

	if p.curToken.Type == TokenDataset {
		return p.parseParseDataset()
	}

	return p.parseParseDocs()
}

func (p *Parser) parseParseDataset() (*Command, error) {
	p.nextToken() // consume DATASET
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	var method string
	if p.curToken.Type == TokenSync {
		method = "sync"
	} else if p.curToken.Type == TokenAsync {
		method = "async"
	} else {
		return nil, fmt.Errorf("expected SYNC or ASYNC")
	}

	cmd := NewCommand("parse_dataset")
	cmd.Params["dataset_name"] = datasetName
	cmd.Params["method"] = method

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseParseDocs() (*Command, error) {
	documentNames, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF")
	}
	p.nextToken()
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET")
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("parse_dataset_docs")
	cmd.Params["document_names"] = documentNames
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseBenchmarkCommand() (*Command, error) {
	cmd := NewCommand("benchmark")

	p.nextToken() // consume BENCHMARK
	concurrency, err := p.parseNumber()
	if err != nil {
		return nil, err
	}
	cmd.Params["concurrency"] = concurrency

	p.nextToken()
	iterations, err := p.parseNumber()
	if err != nil {
		return nil, err
	}
	cmd.Params["iterations"] = iterations

	p.nextToken()
	// Parse user_statement
	nestedCmd, err := p.parseUserStatement()
	if err != nil {
		return nil, err
	}
	cmd.Params["command"] = nestedCmd

	return cmd, nil
}

func (p *Parser) parseUserStatement() (*Command, error) {
	switch p.curToken.Type {
	case TokenPing:
		return p.parsePingServer()
	case TokenShow:
		return p.parseShowCommand()
	case TokenCreate:
		return p.parseCreateCommand()
	case TokenDrop:
		return p.parseDropCommand()
	case TokenSet:
		return p.parseSetCommand()
	case TokenReset:
		return p.parseResetCommand()
	case TokenList:
		return p.parseListCommand()
	case TokenParse:
		return p.parseParseCommand()
	case TokenImport:
		return p.parseImportCommand()
	case TokenSearch:
		return p.parseSearchCommand()
	default:
		return nil, fmt.Errorf("invalid user statement: %s", p.curToken.Value)
	}
}

func (p *Parser) parseStartupCommand() (*Command, error) {
	p.nextToken() // consume STARTUP
	if p.curToken.Type != TokenService {
		return nil, fmt.Errorf("expected SERVICE")
	}
	p.nextToken()

	serviceNum, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("startup_service")
	cmd.Params["number"] = serviceNum

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseShutdownCommand() (*Command, error) {
	p.nextToken() // consume SHUTDOWN
	if p.curToken.Type != TokenService {
		return nil, fmt.Errorf("expected SERVICE")
	}
	p.nextToken()

	serviceNum, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("shutdown_service")
	cmd.Params["number"] = serviceNum

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseRestartCommand() (*Command, error) {
	p.nextToken() // consume RESTART
	if p.curToken.Type != TokenService {
		return nil, fmt.Errorf("expected SERVICE")
	}
	p.nextToken()

	serviceNum, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("restart_service")
	cmd.Params["number"] = serviceNum

	p.nextToken()
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func tokenTypeToString(t int) string {
	// Simplified for error messages
	return fmt.Sprintf("token(%d)", t)
}
