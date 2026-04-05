package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// Command parsers
func (p *Parser) parseLogout() (*Command, error) {
	cmd := NewCommand("logout")
	p.nextToken()
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

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
	// Optional: WITH PASSWORD 'password'
	if p.curToken.Type == TokenWith {
		p.nextToken()
		if p.curToken.Type != TokenPassword {
			return nil, fmt.Errorf("expected PASSWORD after WITH")
		}
		p.nextToken()
		password, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["password"] = password
		p.nextToken()
	}

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parsePingServer() (*Command, error) {
	cmd := NewCommand("ping")
	p.nextToken()
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseRegisterCommand() (*Command, error) {
	cmd := NewCommand("register_user")

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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseListCommand() (*Command, error) {
	p.nextToken() // consume LIST

	switch p.curToken.Type {
	case TokenServices:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("list_services"), nil
	case TokenUsers:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("list_users"), nil
	case TokenRoles:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("list_roles"), nil
	case TokenVars:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("list_variables"), nil
	case TokenConfigs:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("list_configs"), nil
	case TokenEnvs:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("list_environments"), nil
	case TokenDatasets:
		return p.parseListDatasets()
	case TokenAgents:
		return p.parseListAgents()
	case TokenTokens:
		return p.parseListTokens()
	case TokenModel:
		return p.parseListModelProviders()
	case TokenModels:
		return p.parseListModelsOfProvider()
	case TokenProviders:
		return p.parseListProviders()
	case TokenInstances:
		return p.parseListInstances()
	case TokenDefault:
		return p.parseListDefaultModels()
	case TokenAvailable:
		return p.parseCommonListProviders()
	case TokenChats:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
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

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseListTokens() (*Command, error) {
	p.nextToken() // consume TOKENS
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF")
	}
	p.nextToken()

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("list_tokens")
	cmd.Params["user_name"] = userName

	p.nextToken()
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseListModelProviders() (*Command, error) {
	p.nextToken() // consume MODEL
	if p.curToken.Type != TokenProviders {
		return nil, fmt.Errorf("expected PROVIDERS")
	}
	p.nextToken()
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("list_user_model_providers"), nil
}

// parseListProviders parses LIST PROVIDERS command
func (p *Parser) parseListProviders() (*Command, error) {
	p.nextToken() // consume PROVIDERS
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("list_providers"), nil
}

func (p *Parser) parseListDefaultModels() (*Command, error) {
	p.nextToken() // consume DEFAULT
	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS")
	}
	p.nextToken()
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseShowCommand() (*Command, error) {
	p.nextToken() // consume SHOW

	switch p.curToken.Type {
	case TokenVersion:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("show_version"), nil
	case TokenToken:
		p.nextToken()
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("show_token"), nil
	case TokenCurrent:
		p.nextToken()
		if p.curToken.Type == TokenUser {
			p.nextToken()
			// Semicolon is optional for SHOW CURRENT USER
			if p.curToken.Type == TokenSemicolon {
				p.nextToken()
			}
			return NewCommand("show_current_user"), nil
		} else if p.curToken.Type == TokenModel {
			p.nextToken()
			// Semicolon is optional for SHOW CURRENT MODEL
			if p.curToken.Type == TokenSemicolon {
				p.nextToken()
			}
			return NewCommand("show_current_model"), nil
		} else {
			return nil, fmt.Errorf("expected USER or MODEL after CURRENT")
		}
	case TokenUser:
		return p.parseShowUser()
	case TokenRole:
		return p.parseShowRole()
	case TokenVar:
		return p.parseShowVariable()
	case TokenService:
		return p.parseShowService()
	case TokenProvider:
		return p.parseShowProvider()
	case TokenModel:
		return p.parseShowModel()
	case TokenInstance:
		return p.parseShowInstance()
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
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseShowModel() (*Command, error) {
	p.nextToken() // consume model

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}

	cmd := NewCommand("show_model")
	cmd.Params["model_name"] = modelName

	p.nextToken() // consume model_name

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken() // consume from
	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}
	cmd.Params["provider_name"] = providerName
	p.nextToken() // consume provider name
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseShowProvider parses SHOW PROVIDER <name> command
func (p *Parser) parseShowProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}

	cmd := NewCommand("show_provider")
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	case TokenToken:
		return p.parseCreateToken()
	case TokenIndex:
		return p.parseCreateIndex()
	case TokenProvider:
		return p.parseCreateProviderInstance()
	default:
		return nil, fmt.Errorf("unknown CREATE target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAddCommand() (*Command, error) {
	p.nextToken() // consume ADD
	switch p.curToken.Type {
	case TokenProvider:
		return p.parseAddProvider()
	default:
		return nil, fmt.Errorf("unknown ADD target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseCreateToken() (*Command, error) {
	p.nextToken() // consume TOKEN

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("create_token"), nil
}

func (p *Parser) parseCreateIndex() (*Command, error) {
	// CREATE INDEX FOR DATASET 'name' VECTOR_SIZE N
	// CREATE INDEX DOC_META
	p.nextToken() // consume INDEX

	// Check if creating doc meta index
	if p.curToken.Type == TokenDocMeta {
		p.nextToken()
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return NewCommand("create_doc_meta_index"), nil
	}

	// Otherwise, must be CREATE INDEX FOR DATASET 'name' VECTOR_SIZE N
	if p.curToken.Type != TokenFor {
		return nil, fmt.Errorf("expected FOR or DOC_META after INDEX, got %s", p.curToken.Value)
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after FOR, got %s", p.curToken.Value)
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset name, got %s", p.curToken.Value)
	}

	p.nextToken()
	if p.curToken.Type != TokenVectorSize {
		return nil, fmt.Errorf("expected VECTOR_SIZE after dataset name, got %s", p.curToken.Value)
	}
	p.nextToken()

	if p.curToken.Type != TokenNumber {
		return nil, fmt.Errorf("expected vector size number, got %s", p.curToken.Value)
	}
	vectorSize, err := strconv.Atoi(p.curToken.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid vector size: %s", p.curToken.Value)
	}

	p.nextToken()
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("create_index")
	cmd.Params["dataset_name"] = datasetName
	cmd.Params["vector_size"] = vectorSize
	return cmd, nil
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseAddProvider parses ADD PROVIDER commands
// ADD PROVIDER <name>
// ADD PROVIDER <name> <api_key>
func (p *Parser) parseAddProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}

	cmd := NewCommand("add_provider")
	cmd.Params["provider_name"] = providerName

	p.nextToken()

	// Check if api_key is provided (optional)
	if p.curToken.Type == TokenQuotedString {
		apiKey, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected api key: %w", err)
		}
		cmd.Params["api_key"] = apiKey
		p.nextToken()
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	case TokenToken:
		return p.parseDropToken()
	case TokenIndex:
		return p.parseDropIndex()
	case TokenInstance:
		return p.parseDropInstance()
	default:
		return nil, fmt.Errorf("unknown DROP target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseDeleteCommand() (*Command, error) {
	p.nextToken() // consume DELETE

	switch p.curToken.Type {
	case TokenProvider:
		return p.parseDeleteProvider()
	default:
		return nil, fmt.Errorf("unknown DROP target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseDropToken() (*Command, error) {
	p.nextToken() // consume TOKEN

	tokenValue, err := p.parseQuotedString()
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

	cmd := NewCommand("drop_token")
	cmd.Params["token"] = tokenValue
	cmd.Params["user_name"] = userName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseDropIndex() (*Command, error) {
	// DROP INDEX FOR DATASET 'name' OR DROP INDEX DOC_META
	p.nextToken() // consume INDEX

	// Check if dropping doc meta index
	if p.curToken.Type == TokenDocMeta {
		p.nextToken()
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		cmd := NewCommand("drop_doc_meta_index")
		return cmd, nil
	}

	// Otherwise, must be DROP INDEX FOR DATASET 'name'
	if p.curToken.Type != TokenFor {
		return nil, fmt.Errorf("expected FOR or DOC_META after INDEX, got %s", p.curToken.Value)
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after FOR, got %s", p.curToken.Value)
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset name, got %s", p.curToken.Value)
	}

	p.nextToken()
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("drop_index")
	cmd.Params["dataset_name"] = datasetName
	return cmd, nil
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseDeleteProvider parses DELETE PROVIDER <name> command
func (p *Parser) parseDeleteProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}

	cmd := NewCommand("delete_provider")
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	case TokenProvider:
		return p.parseAlterProvider()
	case TokenInstance:
		return p.parseAlterInstance()
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
		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Accept 'on' or 'off' as identifier
	status := p.curToken.Value
	if status != "on" && status != "off" {
		return nil, fmt.Errorf("expected 'on' or 'off', got %s", p.curToken.Value)
	}

	cmd := NewCommand("activate_user")
	cmd.Params["user_name"] = userName
	cmd.Params["activate_status"] = status

	p.nextToken()
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseAlterProvider parses ALTER PROVIDER <name> NAME <new_name> command
func (p *Parser) parseAlterProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}

	p.nextToken()
	if p.curToken.Type != TokenName {
		return nil, fmt.Errorf("expected NAME")
	}
	p.nextToken()

	newName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected new provider name: %w", err)
	}

	cmd := NewCommand("alter_provider")
	cmd.Params["provider_name"] = providerName
	cmd.Params["new_name"] = newName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseCreateProviderInstance parses CREATE PROVIDER <name> INSTANCE <instance_name> <api_key> command
// instance_name cannot be "default"
func (p *Parser) parseCreateProviderInstance() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}

	p.nextToken()
	if p.curToken.Type != TokenInstance {
		return nil, fmt.Errorf("expected INSTANCE after provider name")
	}
	p.nextToken()

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name: %w", err)
	}

	// Check if instance_name is "default"
	if instanceName == "default" {
		return nil, fmt.Errorf("instance name cannot be 'default'")
	}

	p.nextToken()
	apiKey, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected API key: %w", err)
	}

	cmd := NewCommand("create_provider_instance")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	cmd.Params["api_key"] = apiKey

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseListInstances parses LIST INSTANCES FROM PROVIDER <name> command
func (p *Parser) parseListInstances() (*Command, error) {
	p.nextToken() // consume INSTANCES

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name after FROM PROVIDER: %w", err)
	}

	cmd := NewCommand("list_provider_instances")
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseShowInstance parses SHOW INSTANCE <name> FROM PROVIDER <name> command
func (p *Parser) parseShowInstance() (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name: %w", err)
	}

	p.nextToken()
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name after FROM PROVIDER: %w", err)
	}

	cmd := NewCommand("show_provider_instance")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseAlterInstance parses ALTER INSTANCE <name> NAME <new_name> FROM PROVIDER <name> command
func (p *Parser) parseAlterInstance() (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name: %w", err)
	}

	p.nextToken()
	if p.curToken.Type != TokenName {
		return nil, fmt.Errorf("expected NAME")
	}
	p.nextToken()

	newName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected new instance name: %w", err)
	}

	p.nextToken()
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	if p.curToken.Type != TokenProvider {
		return nil, fmt.Errorf("expected PROVIDER after FROM")
	}
	p.nextToken()

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name after FROM PROVIDER: %w", err)
	}

	cmd := NewCommand("alter_provider_instance")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["new_name"] = newName
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseDropInstance parses DROP INSTANCE <name> FROM PROVIDER <name> command
func (p *Parser) parseDropInstance() (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name: %w", err)
	}

	p.nextToken()
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name after FROM PROVIDER: %w", err)
	}

	cmd := NewCommand("drop_provider_instance")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	if p.curToken.Type == TokenToken {
		return p.parseSetToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseSetToken() (*Command, error) {
	p.nextToken() // consume TOKEN

	tokenValue, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("set_token")
	cmd.Params["token"] = tokenValue

	p.nextToken()
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseGenerateCommand() (*Command, error) {
	p.nextToken() // consume GENERATE
	if p.curToken.Type != TokenToken {
		return nil, fmt.Errorf("expected TOKEN")
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

	cmd := NewCommand("generate_token")
	cmd.Params["user_name"] = userName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseInsertCommand parses INSERT command and dispatches to specific handler
func (p *Parser) parseInsertCommand() (*Command, error) {
	p.nextToken() // consume INSERT

	// Expect DATASET or METADATA
	if p.curToken.Type == TokenDataset {
		return p.parseInsertDatasetFromFile()
	}
	if p.curToken.Type == TokenMetadata {
		return p.parseInsertMetadataFromFile()
	}
	return nil, fmt.Errorf("expected DATASET or METADATA after INSERT, got %s", p.curToken.Value)
}

// Internal CLI for GO
// parseInsertDatasetFromFile parses: INSERT DATASET FROM FILE "file_path"
func (p *Parser) parseInsertDatasetFromFile() (*Command, error) {
	p.nextToken() // consume DATASET

	// Expect FROM
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect FILE
	if p.curToken.Type != TokenFile {
		return nil, fmt.Errorf("expected FILE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Get file path (quoted string)
	filePath, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("insert_dataset_from_file")
	cmd.Params["file_path"] = filePath

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// Internal CLI for GO
// parseInsertMetadataFromFile parses: INSERT INTO METADATA FROM FILE "file_path"
func (p *Parser) parseInsertMetadataFromFile() (*Command, error) {
	p.nextToken() // consume METADATA

	// Expect FROM
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect FILE
	if p.curToken.Type != TokenFile {
		return nil, fmt.Errorf("expected FILE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Get file path (quoted string)
	filePath, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("insert_metadata_from_file")
	cmd.Params["file_path"] = filePath

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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

	if p.curToken.Type == TokenOn {
		p.nextToken() // skip on

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
		// Semicolon is optional for UNSET TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	}

	cmd := NewCommand("context_search")

	cmd.Params["query"] = question

	if p.curToken.Type == TokenEOF {
		cmd.Params["path"] = "."
		return cmd, nil
	}

	for p.curToken.Type != TokenEOF {
		if p.curToken.Type == TokenDash {
			p.nextToken() // skip dash
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect identifier")
			}

			if strings.ToLower(p.curToken.Value) == "n" {
				p.nextToken()
				var err error
				if p.curToken.Type != TokenNumber {
					return nil, fmt.Errorf("expect number")
				}
				cmd.Params["number"], err = p.parseNumber()
				if err != nil {
					return nil, err
				}
				p.nextToken()
				continue
			}

			if strings.ToLower(p.curToken.Value) == "t" {
				p.nextToken()
				var err error
				if p.curToken.Type != TokenNumber {
					return nil, fmt.Errorf("expect number")
				}
				cmd.Params["threshold"], err = p.parseFloat()
				if err != nil {
					return nil, err
				}
				p.nextToken()
				continue
			}

			return nil, fmt.Errorf("unknow parameter: %s", p.curToken.Value)
		} else if p.curToken.Type == TokenIdentifier {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = p.curToken.Value
			} else {
				cmd.Params["path"] = fmt.Sprintf("%s %s", cmd.Params["path"], p.curToken.Value)
			}
			p.nextToken() // skip path
			continue
		}
		return nil, fmt.Errorf("syntax error")
	}
	return cmd, nil
}

func (p *Parser) parseListModelsOfProvider() (*Command, error) {
	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS")
	}

	p.nextToken()
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	// Parse first quoted string (could be instance_name or provider_name)
	firstName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	// Check if there's a second quoted string (provider_name)
	// If so, format is: LIST MODELS FROM <instance_name> <provider_name>
	// If not, format is: LIST MODELS FROM <provider_name>
	if p.curToken.Type == TokenQuotedString {
		// Two arguments: instance_name and provider_name
		instanceName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd := NewCommand("list_instance_models")
		cmd.Params["instance_name"] = instanceName
		cmd.Params["provider_name"] = firstName
		p.nextToken()
		// Semicolon is optional for UNSET TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	}

	// Only one argument: provider_name
	cmd := NewCommand("list_provider_models")
	cmd.Params["provider_name"] = firstName
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseEnableCommand() (*Command, error) {
	p.nextToken() // consume ENABLE

	if p.curToken.Type != TokenModel {
		return nil, fmt.Errorf("expected MODEL")
	}
	p.nextToken()

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	modelProvider, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	modelInstance, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("enable_model")
	cmd.Params["model_name"] = modelName
	cmd.Params["instance_name"] = modelInstance
	cmd.Params["provider_name"] = modelProvider
	return cmd, nil
}

func (p *Parser) parseDisableCommand() (*Command, error) {
	p.nextToken() // consume DISABLE

	if p.curToken.Type != TokenModel {
		return nil, fmt.Errorf("expected MODEL")
	}
	p.nextToken()

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	modelProvider, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	modelInstance, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("disable_model")
	cmd.Params["model_name"] = modelName
	cmd.Params["instance_name"] = modelInstance
	cmd.Params["provider_name"] = modelProvider
	return cmd, nil
}

func (p *Parser) parseChatCommand() (*Command, error) {
	p.nextToken() // consume CHAT

	var modelName string
	var message string

	// Check if we have a quoted string that looks like a model identifier (contains two slashes)
	// Format: 'provider/instance/model' or just 'message'
	if p.curToken.Type == TokenQuotedString {
		firstArg := p.curToken.Value

		// Check if it looks like a model identifier (contains exactly 2 slashes)
		slashCount := strings.Count(firstArg, "/")
		if slashCount == 2 {
			// This is likely a model identifier, expect another quoted string for message
			modelName = firstArg
			p.nextToken()

			// After model name, expect message
			if p.curToken.Type != TokenQuotedString {
				return nil, fmt.Errorf("expected message after model name")
			}
			message = p.curToken.Value
			p.nextToken()
		} else {
			// This is just a message, use current model
			message = firstArg
			p.nextToken()
		}
	} else if p.curToken.Type == TokenIdentifier {
		// Context engine style: chat <message>
		message = p.curToken.Value
		p.nextToken()
	} else {
		return nil, fmt.Errorf("expected model name (quoted string) or message")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("chat_to_model")
	if modelName != "" {
		cmd.Params["model_name"] = modelName
	}
	cmd.Params["message"] = message
	cmd.Params["reasoning"] = false
	return cmd, nil
}

func (p *Parser) parseThinkCommand() (*Command, error) {

	p.nextToken() // consume THINK
	command, err := p.parseChatCommand()
	if err != nil {
		return nil, err
	}
	command.Type = "think_chat_to_model"
	command.Params["reasoning"] = true
	return command, nil
}

func (p *Parser) parseUseCommand() (*Command, error) {
	p.nextToken() // consume USE

	if p.curToken.Type != TokenModel {
		return nil, fmt.Errorf("expected MODEL after USE")
	}
	p.nextToken() // consume MODEL

	// Parse model identifier in format 'provider/instance/model'
	modelIdentifier, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model identifier in format 'provider/instance/model': %w", err)
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("use_model")
	cmd.Params["model_identifier"] = modelIdentifier
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	nestedCmd, err := p.parseUserStatement() // Not only user statement
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
	case TokenUnset:
		return p.parseUnsetCommand()
	case TokenReset:
		return p.parseResetCommand()
	case TokenList:
		return p.parseListCommand()
	case TokenParse:
		return p.parseParseCommand()
	case TokenImport:
		return p.parseImportCommand()
	case TokenInsert:
		return p.parseInsertCommand()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
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
	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseUnsetCommand() (*Command, error) {
	p.nextToken() // consume UNSET

	if p.curToken.Type != TokenToken {
		return nil, fmt.Errorf("expected TOKEN after UNSET")
	}
	p.nextToken()

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("unset_token"), nil
}

func (p *Parser) parseContextListCommand() (*Command, error) {
	p.nextToken() // consume LS

	cmd := NewCommand("context_list")

	if p.curToken.Type == TokenEOF {
		cmd.Params["path"] = "."
		return cmd, nil
	}

	for p.curToken.Type != TokenEOF {
		if p.curToken.Type == TokenDash {
			p.nextToken() // skip dash
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect identifier")
			}
			if cmd.Params["parameter"] == nil {
				cmd.Params["parameter"] = p.curToken.Value
			} else {
				cmd.Params["parameter"] = fmt.Sprintf("%s%s", cmd.Params["parameter"], p.curToken.Value)
			}
			p.nextToken() // skip parameter
		} else if p.curToken.Type == TokenIdentifier {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = p.curToken.Value
			} else {
				err := fmt.Errorf("ls: cannot access '%s': No such file or directory", p.curToken.Value)
				return nil, err
			}
			p.nextToken() // skip path
		} else {
			return nil, fmt.Errorf("syntax error")
		}
	}

	return cmd, nil
}

func (p *Parser) parseContextCatCommand() (*Command, error) {
	p.nextToken() // consume CAT

	if p.curToken.Type == TokenEOF {
		return nil, fmt.Errorf("expect a filename")
	}

	if p.curToken.Type != TokenIdentifier && p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expect a filename")
	}

	cmd := NewCommand("context_cat")
	if p.curToken.Type == TokenIdentifier {
		for p.curToken.Type != TokenEOF {
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect a identifier")
			}

			if cmd.Params["filename"] == nil {
				cmd.Params["filename"] = p.curToken.Value
			} else {
				cmd.Params["filename"] = fmt.Sprintf("%s/%s", cmd.Params["filename"], p.curToken.Value)
			}
			p.nextToken()
			if p.curToken.Type == TokenEOF {
				break
			}
			if p.curToken.Type != TokenSlash {
				return nil, fmt.Errorf("expect a slash")
			}
			p.nextToken()
			if p.curToken.Type == TokenEOF {
				return nil, fmt.Errorf("error format")
			}
		}

	} else if p.curToken.Type == TokenQuotedString {
		var err error
		cmd.Params["filename"], err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
	}
	p.nextToken()

	if p.curToken.Type != TokenEOF {
		return nil, fmt.Errorf("syntax error")
	}

	return cmd, nil
}

func (p *Parser) parseContextSearchCommand() (*Command, error) {
	p.nextToken() // consume SERACH

	cmd := NewCommand("context_search")

	for p.curToken.Type != TokenEOF {
		if p.curToken.Type == TokenDash {
			p.nextToken() // skip dash
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect identifier")
			}

			if strings.ToLower(p.curToken.Value) == "n" {
				p.nextToken()
				var err error
				if p.curToken.Type != TokenNumber {
					return nil, fmt.Errorf("expect number")
				}
				cmd.Params["number"], err = p.parseNumber()
				if err != nil {
					return nil, err
				}
				p.nextToken()
				continue
			}

			if strings.ToLower(p.curToken.Value) == "t" {
				p.nextToken()
				var err error
				if p.curToken.Type != TokenNumber {
					return nil, fmt.Errorf("expect number")
				}
				cmd.Params["threshold"], err = p.parseFloat()
				if err != nil {
					return nil, err
				}
				p.nextToken()
				continue
			}

			return nil, fmt.Errorf("unknow parameter: %s", p.curToken.Value)
		} else if p.curToken.Type == TokenIdentifier {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = p.curToken.Value
			} else {
				cmd.Params["path"] = fmt.Sprintf("%s %s", cmd.Params["path"], p.curToken.Value)
			}
			p.nextToken() // skip path
		} else if p.curToken.Type == TokenQuotedString {
			if cmd.Params["query"] == nil {
				var err error
				cmd.Params["query"], err = p.parseQuotedString()
				if err != nil {
					return nil, err
				}
				p.nextToken()
			} else {
				return nil, fmt.Errorf("Query phrase exists")
			}
		}
		return nil, fmt.Errorf("syntax error")
	}

	return cmd, nil
}
