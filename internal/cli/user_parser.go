package cli

import (
	"fmt"
	"strconv"
	"strings"
)

func tokenTypeDescription(t int, tok Token) string {
	if tok.Type == t && tok.Value != "" {
		return fmt.Sprintf("%s %q", tokenTypeToString(t), tok.Value)
	}
	return tokenTypeToString(t)
}

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
	// Optional: PASSWORD 'password'
	if p.curToken.Type == TokenPassword {
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
	p.nextToken() // consume USER

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd.Params["user_name"] = userName
	p.nextToken() // consume Email

	if p.curToken.Type != TokenAs {
		return nil, fmt.Errorf("expected AS")
	}
	p.nextToken() // consume AS

	nickname, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd.Params["nickname"] = nickname
	p.nextToken() // consume nickname

	if p.curToken.Type != TokenPassword {
		return nil, fmt.Errorf("expected PASSWORD")
	}
	p.nextToken() // consume PASSWORD

	password, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd.Params["password"] = password
	p.nextToken() // consume 'password'

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseListCommand() (*Command, error) {
	p.nextToken() // consume LIST

	switch p.curToken.Type {
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
	case TokenDocuments:
		return p.parseListDatasetDocuments()
	case TokenAgents:
		return p.parseListAgents()
	case TokenTokens:
		return p.parseListTokens()
	case TokenModel:
		return p.parseListModelProviders()
	case TokenSupported:
		return p.parseListModelsOfProvider()
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
	case TokenQuotedString:
		return p.parseListQuotedStringCommand()
	case TokenAPI:
		return p.parseListApiCommand()
	default:
		return nil, fmt.Errorf("unknown LIST target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseListDatasets() (*Command, error) {
	cmd := NewCommand("list_datasets")
	p.nextToken() // consume DATASETS

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseListDatasetDocuments() (*Command, error) {
	p.nextToken() // consume DOCUMENTS

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	datasetID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("list_dataset_documents")
	cmd.Params["dataset_id"] = datasetID

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseGetMetadata() (*Command, error) {
	p.nextToken() // consume METADATA

	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after METADATA")
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after OF")
	}
	p.nextToken()

	// Parse dataset names (space-separated)
	var datasetNames []string
	for {
		name, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected dataset name: %w", err)
		}
		datasetNames = append(datasetNames, name)

		p.nextToken()

		if p.curToken.Type == TokenComma {
			return nil, fmt.Errorf("syntax error: dataset names must be space-separated, not comma-separated (got %q after %q)", "'", name)
		}
		// Stop at semicolon or non-quoted (dataset name must be quoted)
		if p.curToken.Type == TokenSemicolon {
			break
		}
		// If next token is not a quoted string, stop parsing dataset names
		if p.curToken.Type != TokenQuotedString {
			break
		}
	}

	cmd := NewCommand("get_metadata")
	cmd.Params["dataset_names"] = datasetNames

	// Semicolon is optional
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
	cmd := NewCommand("list_tokens")
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

func (p *Parser) parseListQuotedStringCommand() (*Command, error) {
	str, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume str
	switch p.curToken.Type {
	case TokenTasks:
		p.nextToken() // consume TASKS
		cmd := NewCommand("list_tasks_user_command")
		cmd.Params["composite_instance_name"] = str
		return cmd, nil
	default:
		return nil, fmt.Errorf("unknown command: %s", str)
	}
}

func (p *Parser) parseShowQuotedStringCommand() (*Command, error) {
	str, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume str
	switch p.curToken.Type {
	case TokenTask:
		p.nextToken() // consume TASK

		var taskID string
		taskID, err = p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected string: %w", err)
		}
		p.nextToken()

		cmd := NewCommand("show_task_user_command")
		cmd.Params["task_id"] = taskID
		cmd.Params["composite_instance_name"] = str
		p.nextToken()

		// Semicolon is optional
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	default:
		return nil, fmt.Errorf("unknown command: %s", str)
	}
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

		// Semicolon is optional for SHOW TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}

		return NewCommand("show_current"), nil
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
	case TokenBalance:
		return p.parseShowBalance()
	case TokenTask:
		return p.parseShowTask()
	case TokenQuotedString:
		return p.parseShowQuotedStringCommand()
	case TokenAdmin:
		return p.parseUserShowAdmin()
	case TokenAPI:
		return p.parseUserShowAPI()
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
	p.nextToken() // consume model_name

	if p.curToken.Type != TokenFrom {
		// SHOW MODEL 'model_name'
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		cmd := NewCommand("show_model")
		cmd.Params["model_name"] = modelName
		return cmd, nil
	}
	p.nextToken() // consume from

	cmd := NewCommand("show_provider_model")
	cmd.Params["model_name"] = modelName

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

// parseListModels parses LIST MODELS
func (p *Parser) parseListAllModels() (*Command, error) {
	p.nextToken() // consume models

	cmd := NewCommand("list_all_models")

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
	case TokenChunkStore:
		return p.parseCreateChunkStore()
	case TokenMetadata:
		return p.parseCreateMetadataStore()
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
	case TokenModel:
		return p.parseAddModel()
	case TokenAPI:
		return p.parseAddAPIServer()
	case TokenAdmin:
		return p.parseAddAdminServer()
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

// Internal CLI for GO
// parseCreateChunkStore parses: CREATE CHUNK STORE for Dataset 'name' VECTOR SIZE N
func (p *Parser) parseCreateChunkStore() (*Command, error) {
	p.nextToken() // consume CHUNK STORE compound token

	// Expect FOR
	if p.curToken.Type != TokenFor {
		return nil, fmt.Errorf("expected FOR after CHUNK STORE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect Dataset
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected Dataset after FOR, got %s", p.curToken.Value)
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset name, got %s", p.curToken.Value)
	}

	p.nextToken()
	if p.curToken.Type != TokenVector {
		return nil, fmt.Errorf("expected VECTOR after dataset name, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type != TokenSize {
		return nil, fmt.Errorf("expected SIZE after VECTOR, got %s", p.curToken.Value)
	}
	p.nextToken()

	if p.curToken.Type != TokenInteger {
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

	cmd := NewCommand("create_chunk_store")
	cmd.Params["dataset_name"] = datasetName
	cmd.Params["vector_size"] = vectorSize
	return cmd, nil
}

// Internal CLI for GO
// parseCreateMetadataStore parses: CREATE METADATA STORE
func (p *Parser) parseCreateMetadataStore() (*Command, error) {
	// CREATE METADATA STORE
	p.nextToken() // consume METADATA

	if p.curToken.Type != TokenStore {
		return nil, fmt.Errorf("expected STORE after METADATA, got %s", p.curToken.Value)
	}
	p.nextToken()

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("create_metadata_store"), nil
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

func (p *Parser) parseModelNames(raw string) ([]string, error) {
	modelNames := strings.Fields(raw)

	if len(modelNames) == 0 {
		return nil, fmt.Errorf("model name is required")
	}

	seen := make(map[string]struct{}, len(modelNames))
	for _, modelName := range modelNames {
		if _, ok := seen[modelName]; ok {
			return nil, fmt.Errorf("duplicate model name: %s", modelName)
		}
		seen[modelName] = struct{}{}
	}

	return modelNames, nil
}

type AddModelConfig struct {
	ModelName  string
	ModelTypes []string
	MaxTokens  int
	Thinking   *bool
}

// syntax: add model 'xxx' to provider 'vllm' instance 'test' with tokens 1024 chat think vision;
func (p *Parser) parseAddModel() (*Command, error) {
	p.nextToken() // consume MODEL

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected model name")
	}

	rawModelNames, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	modelNames, err := p.parseModelNames(rawModelNames)
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume model name

	if p.curToken.Type != TokenTo {
		return nil, fmt.Errorf("expected TO")
	}
	p.nextToken()

	if p.curToken.Type != TokenProvider {
		return nil, fmt.Errorf("expected PROVIDER")
	}
	p.nextToken()

	// provider name
	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected provider name")
	}
	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenInstance {
		return nil, fmt.Errorf("expected INSTANCE")
	}
	p.nextToken()

	// instance name
	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected provider name")
	}
	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	i := 0
	var modelTypes []string
	var supportThink *bool = nil
	maxTokens := 0

	models := make([]map[string]any, 0, len(modelNames))
	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected with")
	}
	p.nextToken()

A:
	for {
		if i >= len(modelNames) {
			return nil, fmt.Errorf("too many model configs: got more configs than model names")
		}
		switch p.curToken.Type {
		case TokenThink:
			if supportThink != nil {
				return nil, fmt.Errorf("think model is already set for model %s", modelNames[i])
			}
			value := true
			supportThink = &value
			p.nextToken()

		case TokenVision:
			modelTypes = append(modelTypes, "vision")
			p.nextToken()

		case TokenChat:
			modelTypes = append(modelTypes, "chat")
			p.nextToken()

		case TokenEmbedding:
			modelTypes = append(modelTypes, "embedding")
			p.nextToken()

		case TokenRerank:
			modelTypes = append(modelTypes, "rerank")
			p.nextToken()

		case TokenOCR:
			modelTypes = append(modelTypes, "ocr")
			p.nextToken()

		case TokenDocParse:
			modelTypes = append(modelTypes, "doc_parse")
			p.nextToken()

		case TokenTTS:
			modelTypes = append(modelTypes, "tts")
			p.nextToken()

		case TokenASR:
			modelTypes = append(modelTypes, "asr")
			p.nextToken()

		case TokenToken, TokenTokens:
			p.nextToken()
			if maxTokens != 0 {
				return nil, fmt.Errorf("max tokens is already given %d for model %s", maxTokens, modelNames[i])
			}
			if p.curToken.Type != TokenInteger {
				return nil, fmt.Errorf("expected integer")
			}
			var err error
			maxTokens, err = p.parseNumber()
			if err != nil {
				return nil, err
			}
			p.nextToken() // consume number

		case TokenComma, TokenSemicolon, TokenEOF:
			if len(modelTypes) == 0 {
				return nil, fmt.Errorf("model type is required for model %s", modelNames[i])
			}

			seenTypes := make(map[string]struct{}, len(modelTypes))
			dedupedModelTypes := make([]string, 0, len(modelTypes))

			for _, modelType := range modelTypes {
				modelType = strings.TrimSpace(modelType)
				if modelType == "" {
					continue
				}

				if _, ok := seenTypes[modelType]; ok {
					continue
				}

				seenTypes[modelType] = struct{}{}
				dedupedModelTypes = append(dedupedModelTypes, modelType)
			}

			modelTypes = dedupedModelTypes
			if len(modelTypes) == 0 {
				return nil, fmt.Errorf("model type is required for model %s", modelNames[i])
			}

			model := map[string]any{
				"model_name":  modelNames[i],
				"model_types": modelTypes,
				"max_tokens":  maxTokens,
			}
			if supportThink != nil {
				model["thinking"] = *supportThink
			}

			models = append(models, model)

			i++
			modelTypes = nil
			supportThink = nil
			maxTokens = 0

			if p.curToken.Type == TokenComma {
				p.nextToken()
				continue
			}

			if p.curToken.Type == TokenSemicolon {
				p.nextToken()
			}
			break A

		default:
			return nil, fmt.Errorf("unexpected token type: %s", p.curToken.Value)
		}

	}
	if len(models) != len(modelNames) {
		return nil, fmt.Errorf("model config count %d does not match model name count %d", len(models), len(modelNames))
	}

	cmd := NewCommand("add_custom_model")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName

	cmd.Params["models"] = models

	return cmd, nil
}

// syntax: add admin server host '127.0.0.1:9333' user 'ccc' password 'ppp'
func (p *Parser) parseAddAdminServer() (*Command, error) {
	p.nextToken() // consume ADMIN

	if p.curToken.Type != TokenServer {
		return nil, fmt.Errorf("expected server name")
	}
	p.nextToken() // consume SERVER

	if p.curToken.Type != TokenHost {
		return nil, fmt.Errorf("expected HOST")
	}
	p.nextToken()

	host, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	ip, port, err := parseHostPort(host)
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("add_admin_server")
	cmd.Params["server_ip"] = ip
	cmd.Params["server_port"] = port

	return cmd, nil
}

// syntax: add api server 'abc' host '127.0.0.1:9333' token 'xxx' user 'ccc' password 'ppp'
func (p *Parser) parseAddAPIServer() (*Command, error) {
	p.nextToken() // consume API

	if p.curToken.Type != TokenServer {
		return nil, fmt.Errorf("expected server name")
	}
	p.nextToken() // consume SERVER

	serverName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume model name

	if p.curToken.Type != TokenHost {
		return nil, fmt.Errorf("expected TO")
	}
	p.nextToken()

	host, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	ip, port, err := parseHostPort(host)
	if err != nil {
		return nil, err
	}

	var token string

optionsLoop:
	for {
		switch p.curToken.Type {
		case TokenToken:
			p.nextToken()
			token, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
		case TokenSemicolon:
			p.nextToken()
			break optionsLoop // done
		default:
			// No more options to process
			break optionsLoop
		}
	}

	cmd := NewCommand("add_api_server")
	cmd.Params["server_name"] = serverName
	cmd.Params["server_ip"] = ip
	cmd.Params["server_port"] = port
	if token != "" {
		cmd.Params["server_token"] = token
	}

	return cmd, nil
}

// syntax: delete api server 'abc'
func (p *Parser) parseDeleteAPIServer() (*Command, error) {
	p.nextToken() // consume API

	if p.curToken.Type != TokenServer {
		return nil, fmt.Errorf("expected server name")
	}
	p.nextToken() // consume SERVER

	serverName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume model name

	cmd := NewCommand("delete_api_server")
	cmd.Params["server_name"] = serverName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// syntax: delete admin server 'abc'
func (p *Parser) parseDeleteAdminServer() (*Command, error) {
	p.nextToken() // consume ADMIN

	if p.curToken.Type != TokenServer {
		return nil, fmt.Errorf("expected server name")
	}
	p.nextToken() // consume SERVER

	cmd := NewCommand("delete_admin_server")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseUserSaveCommand() (*Command, error) {
	p.nextToken() // consume SAVE
	switch p.curToken.Type {
	case TokenConfig:
		return p.parseSaveConfig()
	default:
		return nil, fmt.Errorf("unknown ADD target: %s", p.curToken.Value)
	}
}

// syntax: save config as 'path'
func (p *Parser) parseSaveConfig() (*Command, error) {
	p.nextToken() // consume CONFIG

	if p.curToken.Type != TokenAs {
		return nil, fmt.Errorf("expected AS after CONFIG")
	}
	p.nextToken() // consume AS

	path, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("save_config_command")
	cmd.Params["path"] = path

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
	case TokenDataset:
		return p.parseDropDataset()
	case TokenChat:
		return p.parseDropChat()
	case TokenToken:
		return p.parseDropToken()
	case TokenChunkStore:
		return p.parseDropChunkStore()
	case TokenMetadata:
		return p.parseDropMetadataStore()
	case TokenInstance:
		return p.parseDropInstance()
	case TokenModel:
		return p.parseDropInstanceModel()
	default:
		return nil, fmt.Errorf("unknown DROP target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseDeleteCommand() (*Command, error) {
	p.nextToken() // consume DELETE

	switch p.curToken.Type {
	case TokenProvider:
		return p.parseDeleteProvider()
	case TokenMetadata:
		return p.parseDeleteMeta()
	case TokenAdmin:
		return p.parseDeleteAdminServer()
	case TokenAPI:
		return p.parseDeleteAPIServer()
	default:
		return nil, fmt.Errorf("unknown DELETE target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseRemoveCommand() (*Command, error) {
	p.nextToken() // consume RM

	switch p.curToken.Type {
	case TokenTag:
		return p.parseRemoveTags()
	case TokenChunks, TokenAll:
		return p.parseRemoveChunk()
	case TokenModel:
		return p.parseRemoveInstanceModel()
	default:
		return nil, fmt.Errorf("unknown REMOVE target: %s", p.curToken.Value)
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

// Internal CLI for GO
// parseDropChunkStore parses: DROP CHUNK STORE for Dataset 'name'
func (p *Parser) parseDropChunkStore() (*Command, error) {
	p.nextToken() // consume CHUNK STORE

	// Expect FOR
	if p.curToken.Type != TokenFor {
		return nil, fmt.Errorf("expected FOR after CHUNK STORE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect Dataset
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected Dataset after FOR, got %s", p.curToken.Value)
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

	cmd := NewCommand("drop_chunk_store")
	cmd.Params["dataset_name"] = datasetName
	return cmd, nil
}

// parseDropMetadataStore parses: DROP METADATA STORE
func (p *Parser) parseDropMetadataStore() (*Command, error) {
	// DROP METADATA STORE
	p.nextToken() // consume METADATA

	if p.curToken.Type != TokenStore {
		return nil, fmt.Errorf("expected STORE after METADATA, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("drop_metadata_store")
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

// parseCreateProviderInstance parses CREATE PROVIDER <name> INSTANCE <instance_name> KEY <api_key> URL <base_url> REGION <region> command
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
	p.nextToken()

	if p.curToken.Type != TokenKey {
		return nil, fmt.Errorf("expected KEY after instance name")
	}
	p.nextToken()

	apiKey, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected API key: %w", err)
	}
	p.nextToken()

	baseURL := ""
	region := ""
optionsLoop:
	for {
		switch p.curToken.Type {
		case TokenRegion:
			p.nextToken()
			region, err = p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected region: %w", err)
			}
			p.nextToken()
		case TokenURL:
			p.nextToken()
			baseURL, err = p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected base URL: %w", err)
			}
			p.nextToken()
		default:
			break optionsLoop
		}
	}

	cmd := NewCommand("create_provider_instance")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	cmd.Params["api_key"] = apiKey
	if baseURL != "" {
		// Only local model provider need to set URL
		cmd.Params["base_url"] = baseURL
	}

	if region != "" {
		cmd.Params["region"] = region
	}

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

// parseShowBalance parses SHOW BALANCE FROM <provider_name> <instance_name>
func (p *Parser) parseShowBalance() (*Command, error) {
	p.nextToken() // consume INSTANCE

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected provider name after FROM PROVIDER")
	}
	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name after FROM PROVIDER: %w", err)
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected instance name")
	}
	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name: %w", err)
	}
	p.nextToken()

	cmd := NewCommand("show_instance_balance")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseShowTask parses SHOW TASK <task>
func (p *Parser) parseShowTask() (*Command, error) {
	p.nextToken() // consume TASK

	taskID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected string: %w", err)
	}
	p.nextToken()

	cmd := NewCommand("show_task_user_command")
	cmd.Params["task_id"] = taskID
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

func (p *Parser) parseRemoveInstanceModel() (*Command, error) {
	return p.parseDropInstanceModel()
}

// parseDropInstanceModel parses DROP MODEL <name> FROM <provider_name> <instance_name> command
// Only works for local deployed model
func (p *Parser) parseDropInstanceModel() (*Command, error) {
	p.nextToken() // consume MODEL

	rawModelNames, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	modelNames, err := p.parseModelNames(rawModelNames)
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume model name

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name after FROM PROVIDER: %w", err)
	}
	p.nextToken()

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name after provider name: %w", err)
	}
	p.nextToken()

	cmd := NewCommand("drop_instance_model")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["provider_name"] = providerName
	cmd.Params["model_names"] = modelNames

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
	if p.curToken.Type == TokenMetadata {
		return p.parseSetMeta()
	}
	if p.curToken.Type == TokenLog {
		return p.parseSetLog()
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
	varValue, err := p.parseVariableValue()
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

	var modelType, compositeModelName string
	var err error

	switch p.curToken.Type {
	case TokenChat:
		modelType = "chat"
	case TokenVision:
		modelType = "vision"
	case TokenEmbedding:
		modelType = "embedding"
	case TokenRerank:
		modelType = "rerank"
	case TokenASR:
		modelType = "asr"
	case TokenTTS:
		modelType = "tts"
	case TokenOCR:
		modelType = "ocr"
	default:
		return nil, fmt.Errorf("unknown model type: %s", p.curToken.Value)
	}
	p.nextToken() // pass model type

	if p.curToken.Type != TokenModel {
		return nil, fmt.Errorf("expected MODEL")
	}
	p.nextToken() // pass MODEL

	// Format: 'provider/instance/model' or just 'message'
	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected quoted string with format provider/instance/model")
	}

	compositeModelName, err = p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("set_default_model")
	cmd.Params["model_type"] = modelType
	cmd.Params["composite_model_name"] = compositeModelName

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

func (p *Parser) parseSetLog() (*Command, error) {
	p.nextToken() // consume LOG

	switch p.curToken.Type {
	case TokenLevel:
		return p.parseSetLogLevel()
	default:
		return nil, fmt.Errorf("unknown log target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseSetLogLevel() (*Command, error) {
	p.nextToken() // consume LEVEL

	cmd := NewCommand("set_log_level")
	switch p.curToken.Type {
	case TokenDebug:
		cmd.Params["level"] = "debug"
	case TokenInfo:
		cmd.Params["level"] = "info"
	case TokenWarn:
		cmd.Params["level"] = "warn"
	case TokenError:
		cmd.Params["level"] = "error"
	case TokenFatal:
		cmd.Params["level"] = "fatal"
	case TokenPanic:
		cmd.Params["level"] = "panic"
	default:
		return nil, fmt.Errorf("unknown log target: %s", p.curToken.Value)
	}
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
	case TokenChat:
		modelType = "chat"
	case TokenVision:
		modelType = "vision"
	case TokenEmbedding:
		modelType = "embedding"
	case TokenRerank:
		modelType = "rerank"
	case TokenASR:
		modelType = "asr"
	case TokenTTS:
		modelType = "tts"
	case TokenOCR:
		modelType = "ocr"
	default:
		return nil, fmt.Errorf("unknown model type: %s", p.curToken.Value)
	}

	cmd := NewCommand("reset_default_model")
	cmd.Params["model_type"] = modelType
	p.nextToken()

	if p.curToken.Type != TokenModel {
		return nil, fmt.Errorf("expected MODEL")
	}
	p.nextToken() // pass MODEL

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

	// Expect CHUNKS or METADATA
	if p.curToken.Type == TokenChunks {
		return p.parseInsertChunksFromFile()
	}
	if p.curToken.Type == TokenMetadata {
		return p.parseInsertMetadataFromFile()
	}
	return nil, fmt.Errorf("expected CHUNKS or METADATA after INSERT, got %s", p.curToken.Value)
}

// Internal CLI for GO
// parseInsertChunksFromFile parses: INSERT CHUNKS FROM FILE "file_path"
func (p *Parser) parseInsertChunksFromFile() (*Command, error) {
	p.nextToken() // consume CHUNKS

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

	cmd := NewCommand("insert_chunks_from_file")
	cmd.Params["file_path"] = filePath

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// Internal CLI for GO
// parseInsertMetadataFromFile parses: INSERT METADATA FROM FILE "file_path"
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

func (p *Parser) parseRetrieveCommand() (*Command, error) {
	p.nextToken() // consume SEARCH

	// Handle help flag: -h / --help. The lexer tokenizes each leading
	// `-` as a separate `TokenDash` and then the rest of the flag name
	// (e.g. "help") as a `TokenIdentifier`. We collect any leading
	// dashes before checking the identifier value. Short-circuit with
	// a dedicated command type so the dispatcher can print the search
	// usage instead of erroring out on the missing question.
	if p.curToken.Type == TokenDash {
		dashCount := 0
		for p.curToken.Type == TokenDash {
			dashCount++
			p.nextToken()
		}
		if dashCount > 0 && p.curToken.Type == TokenIdentifier {
			switch strings.ToLower(p.curToken.Value) {
			case "h", "help":
				return NewCommand("search_help"), nil
			}
		}
		return nil, fmt.Errorf("expected quoted string or identifier")
	}

	var err error
	var question string
	if p.curToken.Type == TokenQuotedString {
		question, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
	} else if p.curToken.Type == TokenIdentifier {
		question, err = p.parseIdentifier()
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("expected quoted string or identifier")
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

		// Parse optional WITH clause for additional parameters
		if p.curToken.Type == TokenWith || (p.curToken.Type == TokenIdentifier && strings.ToLower(p.curToken.Value) == "with") {
			if p.curToken.Type == TokenWith {
				p.nextToken()
			} else {
				p.nextToken() // skip "with" identifier
			}

			for p.curToken.Type != TokenEOF && p.curToken.Type != TokenSemicolon {
				if p.curToken.Type == TokenComma {
					return nil, fmt.Errorf("syntax error: WITH options must be space-separated, not comma-separated")
				}
				// Parse parameter name
				if p.curToken.Type != TokenIdentifier {
					break
				}
				paramName := strings.ToLower(p.curToken.Value)
				p.nextToken()

				// Parse parameter value
				var paramValue interface{}
				valueToken := p.curToken.Type
				var valueErr error
				switch p.curToken.Type {
				case TokenInteger:
					paramValue, valueErr = p.parseNumber()
					if valueErr != nil {
						return nil, valueErr
					}
					p.nextToken() // step past the integer
				case TokenFloat:
					paramValue, valueErr = p.parseFloat()
					if valueErr != nil {
						return nil, valueErr
					}
					p.nextToken() // step past the float
				case TokenQuotedString:
					paramValue, valueErr = p.parseQuotedString()
					if valueErr != nil {
						return nil, valueErr
					}
					p.nextToken() // step past the closing quote
				case TokenIdentifier:
					// Bare identifiers are only meaningful for the
					// boolean keys (keyword / use_kg = true|false);
					// everything else rejects them.
					paramValue = p.curToken.Value
					p.nextToken()
				case TokenLBracket:
					// List value: parsed inside the switch below by
					// cross_languages / doc_ids. No value is captured here.
					paramValue = nil
				default:
					// EOF, ';', or any other non-value token: the option
					// is missing a value, which is a hard error rather
					// than a silent drop.
					return nil, fmt.Errorf("WITH option %q is missing a value", paramName)
				}

				switch paramName {
				case "top_k", "page_size", "page":
					if valueToken != TokenInteger {
						return nil, fmt.Errorf("WITH option %q must be an integer, got %s", paramName, tokenTypeDescription(valueToken, p.curToken))
					}
					cmd.Params[paramName] = paramValue
				case "similarity_threshold", "vector_similarity_weight":
					switch n := paramValue.(type) {
					case int:
						cmd.Params[paramName] = float64(n)
					case float64:
						cmd.Params[paramName] = n
					default:
						return nil, fmt.Errorf("WITH option %q must be a number, got %s", paramName, tokenTypeDescription(valueToken, p.curToken))
					}
				case "keyword", "use_kg":
					s, ok := paramValue.(string)
					if !ok {
						return nil, fmt.Errorf("WITH option %q must be true or false, got %s", paramName, tokenTypeDescription(valueToken, p.curToken))
					}
					switch strings.ToLower(s) {
					case "true":
						cmd.Params[paramName] = true
					case "false":
						cmd.Params[paramName] = false
					default:
						return nil, fmt.Errorf("WITH option %q must be true or false, got %q", paramName, s)
					}
				case "rerank_id", "tenant_rerank_id", "search_id", "meta_data_filter":
					if valueToken != TokenQuotedString {
						return nil, fmt.Errorf("WITH option %q must be a quoted string, got %s", paramName, tokenTypeDescription(valueToken, p.curToken))
					}
					// meta_data_filter JSON string is decoded into a map in
					// the SearchOnDatasets handler; parser stores the raw
					// string so the handler can surface a clean error on
					// invalid JSON.
					cmd.Params[paramName] = paramValue
				case "cross_languages", "doc_ids":
					if p.curToken.Type != TokenLBracket {
						return nil, fmt.Errorf("WITH option %q must be a list, e.g. %q ['a', 'b']", paramName, paramName)
					}
					list, err := p.parseQuotedStringList()
					if err != nil {
						return nil, err
					}
					cmd.Params[paramName] = list
				default:
					return nil, fmt.Errorf("unknown WITH option %q", paramName)
				}
			}
		}

		// Semicolon is optional
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	}

	cmd := NewCommand("ce_search")

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
				if p.curToken.Type != TokenInteger {
					return nil, fmt.Errorf("expect number")
				}
				cmd.Params["number"], err = p.parseNumber()
				if err != nil {
					return nil, err
				}
				p.nextToken()
				continue
			}

			//if strings.ToLower(p.curToken.Value) == "t" {
			//	p.nextToken()
			//	var err error
			//	if p.curToken.Type != TokenInteger {
			//		return nil, fmt.Errorf("expect number")
			//	}
			//	cmd.Params["threshold"], err = p.parseFloat()
			//	if err != nil {
			//		return nil, err
			//	}
			//	p.nextToken()
			//	continue
			//}

			return nil, fmt.Errorf("unknow parameter: %s", p.curToken.Value)
		} else if p.curToken.Type == TokenIdentifier {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = p.curToken.Value
			} else {
				cmd.Params["path"] = fmt.Sprintf("%s%s", cmd.Params["path"], p.curToken.Value)
			}
			p.nextToken() // skip path
			continue
		} else if p.curToken.Type == TokenSlash {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = "/"
			} else {
				cmd.Params["path"] = fmt.Sprintf("%s/", cmd.Params["path"])
			}
			p.nextToken() // skip slash
			if p.curToken.Type == TokenIdentifier {
				cmd.Params["path"] = fmt.Sprintf("%s%s", cmd.Params["path"], p.curToken.Value)
				p.nextToken()
			}
			continue
		}
	}
	return cmd, nil
}

func (p *Parser) parseListModelsOfProvider() (*Command, error) {

	if p.curToken.Type == TokenSupported {
		// List supported models
		p.nextToken()

		cmd := NewCommand("list_supported_models")
		if p.curToken.Type != TokenModels {
			return nil, fmt.Errorf("expected MODELS")
		}
		p.nextToken()

		if p.curToken.Type != TokenFrom {
			return nil, fmt.Errorf("expected FROM")
		}
		p.nextToken()

		if p.curToken.Type != TokenQuotedString {
			return nil, fmt.Errorf("expected quoted string for provider name")
		}
		firstName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		p.nextToken()

		if p.curToken.Type != TokenQuotedString {
			return nil, fmt.Errorf("expected quoted string for instance name")
		}
		secondName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		p.nextToken()

		cmd.Params["provider_name"] = firstName
		cmd.Params["instance_name"] = secondName

		// Semicolon is optional for UNSET TOKEN
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	}

	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS")
	}
	p.nextToken()

	if p.curToken.Type != TokenFrom {
		// LIST MODELS
		cmd := NewCommand("list_all_models")

		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
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

// CHAT 'model@instance@provider' 'hello world'
// CHAT WITH 'model@instance@provider' MESSAGE 'hello world' 'who are you' IMAGE 'url1' 'file0' VIDEO "url2.mov" "file1" FILE "url" "path file2" AUDIO "file.wav"
func (p *Parser) parseChatCommand() (*Command, error) {
	p.nextToken() // consume CHAT

	var err error
	var compositeModelName string = ""
	var messages []string
	var images []string
	var videos []string
	var audios []string
	var files []string
	effort := "default"
	verbosity := "low"

optionsLoop:
	for {
		switch p.curToken.Type {
		case TokenWith:
			p.nextToken()
			// 'model@instance@provider'
			if compositeModelName != "" {
				return nil, fmt.Errorf("model name is already set")
			}
			compositeModelName, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			p.nextToken()
		case TokenMessage:
			p.nextToken()
			if len(messages) != 0 {
				return nil, fmt.Errorf("message is already set")
			}
		messageLoop:
			for {
				if p.curToken.Type != TokenQuotedString {
					break messageLoop
				}
				var message string
				message, err = p.parseQuotedString()
				if err != nil {
					return nil, err
				}
				message = strings.TrimSpace(message)
				messages = append(messages, message)
				p.nextToken()
			}
		case TokenImage:
			p.nextToken()
			if len(images) != 0 {
				return nil, fmt.Errorf("image is already set")
			}
		imageLoop:
			for {
				if p.curToken.Type != TokenQuotedString {
					break imageLoop
				}
				var image string
				image, err = p.parseQuotedString()
				if err != nil {
					return nil, err
				}
				images = append(images, image)
				p.nextToken()
			}
		case TokenVideo:
			p.nextToken()
			if len(videos) != 0 {
				return nil, fmt.Errorf("video is already set")
			}
		videoLoop:
			for {
				if p.curToken.Type != TokenQuotedString {
					break videoLoop
				}
				var video string
				video, err = p.parseQuotedString()
				if err != nil {
					return nil, err
				}
				videos = append(videos, video)
				p.nextToken()
			}
		case TokenAudio:
			p.nextToken()
			if len(audios) != 0 {
				return nil, fmt.Errorf("video is already set")
			}
		audioLoop:
			for {
				if p.curToken.Type != TokenQuotedString {
					break audioLoop
				}
				var audio string
				audio, err = p.parseQuotedString()
				if err != nil {
					return nil, err
				}
				audios = append(audios, audio)
				p.nextToken()
			}
		case TokenFile:
			p.nextToken()
			if len(files) != 0 {
				return nil, fmt.Errorf("video is already set")
			}
		fileLoop:
			for {
				if p.curToken.Type != TokenQuotedString {
					break fileLoop
				}
				var file string
				file, err = p.parseQuotedString()
				if err != nil {
					return nil, err
				}
				files = append(files, file)
				p.nextToken()
			}
		case TokenEffort:
			p.nextToken() // pass Effort
			switch p.curToken.Type {
			case TokenNone:
				effort = "none"
			case TokenMinimal:
				effort = "minimal"
			case TokenLow:
				effort = "low"
			case TokenMedium:
				effort = "medium"
			case TokenHigh:
				effort = "high"
			case TokenMax:
				effort = "max"
			default:
				return nil, fmt.Errorf("invalid effort level")
			}
			p.nextToken()
			break optionsLoop
		case TokenVerbosity:
			p.nextToken() // pass VERBOSITY
			switch p.curToken.Type {
			case TokenLow:
				verbosity = "low"
			case TokenMedium:
				verbosity = "median"
			case TokenHigh:
				verbosity = "high"
			default:
				return nil, fmt.Errorf("invalid verbosity level")
			}
			p.nextToken()
			break optionsLoop
		case TokenSemicolon:
			p.nextToken()
			break optionsLoop // done
		default:
			// No more options to process
			break optionsLoop
		}
	}
	cmd := NewCommand("chat_to_model")

	cmd.Params["composite_model_name"] = compositeModelName
	cmd.Params["messages"] = messages
	cmd.Params["images"] = images
	cmd.Params["videos"] = videos
	cmd.Params["audios"] = audios
	cmd.Params["files"] = files
	cmd.Params["thinking"] = false
	cmd.Params["stream"] = false
	cmd.Params["effort"] = effort
	cmd.Params["verbosity"] = verbosity
	return cmd, nil
}

func (p *Parser) parseThinkCommand() (*Command, error) {

	p.nextToken() // consume THINK

	if p.curToken.Type != TokenChat {
		return nil, fmt.Errorf("expected CHAT after THINK")
	}

	command, err := p.parseChatCommand()
	if err != nil {
		return nil, err
	}
	command.Params["thinking"] = true
	return command, nil
}

func (p *Parser) parseStreamCommand() (*Command, error) {

	p.nextToken() // consume STREAM

	var command *Command
	var err error

	switch p.curToken.Type {
	case TokenChat:
		command, err = p.parseChatCommand()
		if err != nil {
			return nil, err
		}
	case TokenThink:
		command, err = p.parseThinkCommand()
		if err != nil {
			return nil, err
		}
	case TokenASR:
		command, err = p.parseASRCommand()
		if err != nil {
			return nil, err
		}
	case TokenTTS:
		command, err = p.parseTTSCommand()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("expected CHAT, THINK, ASR, or TTS after STREAM")
	}

	command.Params["stream"] = true
	return command, nil
}

func (p *Parser) parseEmbedCommand() (*Command, error) {
	p.nextToken() // consume EMBED

	if p.curToken.Type != TokenText {
		return nil, fmt.Errorf("expected WITH after EMBED")
	}
	p.nextToken() // consume TEXT

	var texts []string

textLoop:
	for {
		if p.curToken.Type != TokenQuotedString {
			break textLoop
		}
		text, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		text = strings.TrimSpace(text)
		texts = append(texts, text)
		p.nextToken()
	}

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after EMBED")
	}
	p.nextToken() // consume WITH

	compositeModelName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	dimension := 0
	if p.curToken.Type == TokenDimension {
		p.nextToken() // consume DIMENSION

		if p.curToken.Type != TokenInteger {
			return nil, fmt.Errorf("expected integer after DIMENSION")
		}

		var err error
		dimension, err = p.parseNumber()
		if err != nil {
			return nil, err
		}
		p.nextToken()
	}

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	if p.curToken.Type != TokenEOF {
		return nil, fmt.Errorf("unexpected token after embed command: %s", p.curToken.Value)
	}

	cmd := NewCommand("embed_user_text")
	cmd.Params["composite_model_name"] = compositeModelName
	cmd.Params["texts"] = texts
	if dimension > 0 {
		cmd.Params["dimension"] = dimension
	}
	return cmd, nil
}

func (p *Parser) parseRerankCommand() (*Command, error) {
	p.nextToken() // consume RERANK

	if p.curToken.Type != TokenQuery {
		return nil, fmt.Errorf("expected WITH after EMBED")
	}
	p.nextToken() // consume QUERY

	query, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	p.nextToken() // consume query

	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after query")
	}
	p.nextToken() // consume DOCUMENT

	var documents []string

documentLoop:
	for {
		if p.curToken.Type != TokenQuotedString {
			break documentLoop
		}
		var document string
		document, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		document = strings.TrimSpace(document)
		documents = append(documents, document)
		p.nextToken()
	}

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after EMBED")
	}
	p.nextToken() // consume WITH

	compositeModelName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenTop {
		return nil, fmt.Errorf("expected TOP after model")
	}
	p.nextToken()

	topN, err := p.parseNumber()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("rarank_user_document")
	cmd.Params["composite_model_name"] = compositeModelName
	cmd.Params["query"] = query
	cmd.Params["documents"] = documents
	cmd.Params["top_n"] = topN
	return cmd, nil
}

func (p *Parser) parseASRCommand() (*Command, error) {
	p.nextToken() // consume ASR

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after ASR")
	}
	p.nextToken() // consume WITH

	compositeModelName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenAudio {
		return nil, fmt.Errorf("expected AUDIO to ASR")
	}
	p.nextToken() // consume AUDIO

	audioFile, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("asr_user_command")
	cmd.Params["composite_model_name"] = compositeModelName
	cmd.Params["audio_file"] = audioFile

	for p.curToken.Type != TokenEOF && p.curToken.Type != TokenSemicolon {
		switch p.curToken.Type {
		case TokenParam:
			p.nextToken()
			if p.curToken.Type != TokenQuotedString {
				return nil, fmt.Errorf("expect quoted string after 'param'")
			}
			cmd.Params["param_str"] = strings.Trim(p.curToken.Value, "\"'")
			p.nextToken()
		default:
			return nil, fmt.Errorf("unexpected token in asr command: %s", p.curToken.Value)
		}
	}

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseTTSCommand() (*Command, error) {
	p.nextToken()

	cmd := NewCommand("tts_user_command")

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expect 'with' after tts")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString && p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expect model name after 'with'")
	}
	cmd.Params["composite_model_name"] = strings.Trim(p.curToken.Value, "\"'")
	p.nextToken()

	if p.curToken.Type != TokenText {
		return nil, fmt.Errorf("expect 'text' parameter")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expect quoted string after 'text'")
	}
	cmd.Params["text"] = strings.Trim(p.curToken.Value, "\"'")
	p.nextToken()

	for p.curToken.Type != TokenEOF && p.curToken.Type != TokenSemicolon {
		switch p.curToken.Type {
		case TokenPlay:
			p.nextToken()
			cmd.Params["play"] = true
		case TokenParam:
			p.nextToken()
			if p.curToken.Type != TokenQuotedString {
				return nil, fmt.Errorf("expect quoted string after 'param'")
			}
			cmd.Params["param_str"] = strings.Trim(p.curToken.Value, "\"'")
			p.nextToken()
			p.nextToken()
		case TokenSave:
			p.nextToken()

			if p.curToken.Type != TokenQuotedString && p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect directory path after 'save'")
			}

			cmd.Params["save"] = true
			cmd.Params["save_path"] = strings.Trim(p.curToken.Value, "\"'")
			p.nextToken()
		case TokenFormat:
			p.nextToken()
			if p.curToken.Type != TokenQuotedString && p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect format string (e.g. 'wav') after 'format'")
			}
			cmd.Params["format"] = strings.Trim(p.curToken.Value, "\"'")
			p.nextToken()
		default:
			return nil, fmt.Errorf("unexpected token: %s", p.curToken.Value)
		}
	}

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseOCRCommand() (*Command, error) {
	p.nextToken() // consume OCR

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after OCR")
	}
	p.nextToken() // consume WITH

	compositeModelName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("ocr_user_command")

	switch p.curToken.Type {
	case TokenFile:
		p.nextToken()
		var file string
		file, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["file"] = file
		p.nextToken()
	case TokenURL:
		p.nextToken()
		var url string
		url, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["url"] = url
		p.nextToken()
	default:
		return nil, fmt.Errorf("expected FILE or URL")
	}

	cmd.Params["composite_model_name"] = compositeModelName

	return cmd, nil
}

func (p *Parser) parseModelParseCommand() (*Command, error) {
	p.nextToken() // consume WITH

	compositeModelName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("parse_file_user_command")

	switch p.curToken.Type {
	case TokenFile:
		p.nextToken()
		var file string
		file, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["file"] = file
		p.nextToken()
	case TokenURL:
		p.nextToken()
		var url string
		url, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["url"] = url
		p.nextToken()
	default:
		return nil, fmt.Errorf("expected FILE or URL")
	}

	cmd.Params["composite_model_name"] = compositeModelName

	return cmd, nil
}

func (p *Parser) parseCheckCommand() (*Command, error) {
	p.nextToken() // consume CHECK

	switch p.curToken.Type {
	case TokenInstance:
		return p.parseCheckInstanceCommand()
	case TokenProvider:
		return p.parseCheckProviderByKeyCommand()
	default:
		return nil, fmt.Errorf("expected INSTANCE or PROVIDER after CHECK")
	}
}

func (p *Parser) parseCheckInstanceCommand() (*Command, error) {
	if p.curToken.Type != TokenInstance {
		return nil, fmt.Errorf("expected INSTANCE after CHECK")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected instance name after INSTANCE")
	}
	instanceName := p.curToken.Value
	p.nextToken()

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM after instance name")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected provider name after FROM")
	}
	providerName := p.curToken.Value
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("check_provider_connection")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	return cmd, nil
}

func (p *Parser) parseCheckProviderByKeyCommand() (*Command, error) {
	if p.curToken.Type != TokenProvider {
		return nil, fmt.Errorf("expected PROVIDER after CHECK")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected provider name after PROVIDER")
	}
	providerName := p.curToken.Value
	p.nextToken()

	if p.curToken.Type != TokenRegion {
		return nil, fmt.Errorf("expected REGION after provider name")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected region name after REGION")
	}
	regionName := p.curToken.Value
	p.nextToken()

	if p.curToken.Type != TokenKey {
		return nil, fmt.Errorf("expected KEY after region name")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected API key after KEY")
	}
	apiKey := p.curToken.Value
	p.nextToken()

	baseURL := ""
	if p.curToken.Type == TokenURL {
		p.nextToken()
		if p.curToken.Type != TokenQuotedString {
			return nil, fmt.Errorf("expected base URL after URL")
		}
		baseURL = p.curToken.Value
		p.nextToken()
	}

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	if p.curToken.Type != TokenEOF {
		return nil, fmt.Errorf("unexpected token: %s", p.curToken.Value)
	}

	cmd := NewCommand("check_provider_with_key")
	cmd.Params["provider_name"] = providerName
	cmd.Params["region"] = regionName
	cmd.Params["api_key"] = apiKey
	if baseURL != "" {
		cmd.Params["base_url"] = baseURL
	}

	return cmd, nil
}

func (p *Parser) parseUseCommand() (*Command, error) {
	p.nextToken() // consume USE

	switch p.curToken.Type {
	case TokenModel:
		return p.parseUseModel()
	case TokenAPI:
		return p.parseUseAPIServer()
	case TokenAdmin:
		return p.parseUseAdminServer()
	default:
		return nil, fmt.Errorf("expected MODEL or SKILL after USE")
	}
}

func (p *Parser) parseUseModel() (*Command, error) {
	p.nextToken() // consume MODEL

	compositeModelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model identifier in format 'model@instance@provider': %w", err)
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("use_model")
	cmd.Params["composite_model_name"] = compositeModelName
	return cmd, nil
}

func (p *Parser) parseUseAPIServer() (*Command, error) {
	p.nextToken() // consume API

	serverName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()
	cmd := NewCommand("use_api_server")
	cmd.Params["server_name"] = serverName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseUseAdminServer() (*Command, error) {
	p.nextToken() // consume ADMIN

	cmd := NewCommand("use_admin_server")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseParseCommand() (*Command, error) {
	p.nextToken() // consume PARSE

	switch p.curToken.Type {
	case TokenDataset:
		return p.parseParseDataset()
	case TokenWith:
		return p.parseModelParseCommand()
	case TokenDocument:
		return p.parseParseDocs()
	case TokenFile:
		return p.parseParseLocalFileCommand()
	default:
		return nil, fmt.Errorf("expected DATASET, WITH, or DOCUMENT")
	}
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
	p.nextToken() // consume document

	documentsStr, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	p.nextToken()
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM")
	}
	p.nextToken()

	datasetID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("parse_documents_user_command")

	documents := strings.Split(documentsStr, " ")

	cmd.Params["documents"] = documents
	cmd.Params["dataset_id"] = datasetID

	// Semicolon is optional for UNSET TOKEN
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseParseLocalFileCommand() (*Command, error) {
	p.nextToken() // consume FILE

	filename, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("user_parse_local_file_command")
	cmd.Params["filename"] = filename

optionsLoop:
	for {
		switch p.curToken.Type {
		case TokenVision:
			p.nextToken()
			var visionModel string
			visionModel, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["vision_model"] = visionModel
			p.nextToken()
		case TokenASR:
			p.nextToken()
			var asrModel string
			asrModel, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["asr_model"] = asrModel
			p.nextToken()

		case TokenOCR:
			p.nextToken()
			var ocrModel string
			ocrModel, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["ocr_model"] = ocrModel
			p.nextToken()
		case TokenChat:
			p.nextToken()
			var chatModel string
			chatModel, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["chat_model"] = chatModel
			p.nextToken()
		case TokenEmbed:
			p.nextToken()
			var embedModel string
			embedModel, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["embed_model"] = embedModel
			p.nextToken()
		case TokenDocParse:
			p.nextToken()
			var docParseModel string
			docParseModel, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["doc_parse_model"] = docParseModel
			p.nextToken()
		case TokenSemicolon:
			p.nextToken()
			break optionsLoop // done
		default:
			// No more options to process
			break optionsLoop
		}
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
	case TokenDelete:
		return p.parseDeleteCommand()
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
	case TokenRetrieve:
		return p.parseRetrieveCommand()
	case TokenGet:
		return p.parseGetCommand()
	case TokenUpdate:
		return p.parseUpdateCommand()
	case TokenRemove:
		return p.parseRemoveCommand()
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

// parseGetCommand parses: GET CHUNK or GET METADATA
func (p *Parser) parseGetCommand() (*Command, error) {
	p.nextToken() // consume GET

	if p.curToken.Type == TokenChunk {
		return p.parseGetChunk()
	}
	if p.curToken.Type == TokenMetadata {
		return p.parseGetMetadata()
	}

	return nil, fmt.Errorf("unknown GET target: %s", p.curToken.Value)
}

// parseGetChunk parses: GET CHUNK 'chunk_id' OF DOCUMENT 'doc_id' IN DATASET 'dataset_id'
func (p *Parser) parseGetChunk() (*Command, error) {
	p.nextToken() // consume CHUNK

	// Parse chunk_id
	chunkID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected chunk_id: %w", err)
	}

	cmd := NewCommand("get_chunk")
	cmd.Params["chunk_id"] = chunkID

	p.nextToken()
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after chunk_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd.Params["doc_id"] = docID

	p.nextToken()
	if p.curToken.Type != TokenIn {
		return nil, fmt.Errorf("expected IN after doc_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after IN")
	}
	p.nextToken()

	// Parse dataset_id
	datasetID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_id: %w", err)
	}
	cmd.Params["dataset_id"] = datasetID

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// Internal
// parseUpdateCommand parses: UPDATE CHUNK 'chunk_id' OF DATASET 'dataset_name' SET '{"content": "..."}'
func (p *Parser) parseUpdateCommand() (*Command, error) {
	p.nextToken() // consume UPDATE

	if p.curToken.Type == TokenChunk {
		return p.parseUpdateChunk()
	}

	return nil, fmt.Errorf("unknown UPDATE target: %s", p.curToken.Value)
}

// Internal CLI for GO
// parseUpdateChunk parses: UPDATE CHUNK 'chunk_id' OF DOCUMENT 'doc_id' IN DATASET 'dataset_id' SET '{"content": "..."}'
func (p *Parser) parseUpdateChunk() (*Command, error) {
	p.nextToken() // consume CHUNK

	// Parse chunk_id
	chunkID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected chunk_id: %w", err)
	}

	cmd := NewCommand("update_chunk")
	cmd.Params["chunk_id"] = chunkID

	p.nextToken()
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after chunk_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd.Params["doc_id"] = docID

	p.nextToken()
	if p.curToken.Type != TokenIn {
		return nil, fmt.Errorf("expected IN after doc_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after IN")
	}
	p.nextToken()

	// Parse dataset_name
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_name: %w", err)
	}
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()
	if p.curToken.Type != TokenSet {
		return nil, fmt.Errorf("expected SET after dataset_name")
	}
	p.nextToken()

	// Parse JSON body
	jsonBody, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected JSON body: %w", err)
	}
	cmd.Params["json_body"] = jsonBody

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseSetMeta parses: SET METADATA OF DOCUMENT 'doc_id' TO '{"key": "value"}'
func (p *Parser) parseSetMeta() (*Command, error) {
	p.nextToken() // consume METADATA

	// Expect OF
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after SET METADATA")
	}
	p.nextToken()

	// Expect DOCUMENT
	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after SET METADATA OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd := NewCommand("set_meta")
	cmd.Params["doc_id"] = docID

	p.nextToken()
	// Expect TO
	if p.curToken.Type != TokenTo {
		return nil, fmt.Errorf("expected TO after doc_id")
	}
	p.nextToken()

	// Parse meta JSON
	meta, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected meta JSON: %w", err)
	}
	cmd.Params["meta"] = meta

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseDeleteMeta parses: DELETE METADATA OF DOCUMENT 'doc_id' [KEYS '["key1", "key2"]']
// If KEYS is not provided, deletes entire document metadata
func (p *Parser) parseDeleteMeta() (*Command, error) {
	p.nextToken() // consume METADATA

	// Expect OF
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after DELETE METADATA")
	}
	p.nextToken()

	// Expect DOCUMENT
	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after DELETE METADATA OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd := NewCommand("delete_meta")
	cmd.Params["doc_id"] = docID

	p.nextToken()
	// KEYS is optional - if not provided, delete entire document metadata
	if p.curToken.Type != TokenKeys {
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
			return cmd, nil
		}
		if p.curToken.Type == TokenEOF {
			return cmd, nil
		}
		return nil, fmt.Errorf("expected KEYS or end of command after doc_id")
	}

	// Parse keys JSON array
	p.nextToken()
	keys, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected keys JSON array: %w", err)
	}
	cmd.Params["keys"] = keys

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
		return cmd, nil
	}
	if p.curToken.Type != TokenEOF {
		return nil, fmt.Errorf("expected end of command after KEYS")
	}

	return cmd, nil
}

// parseRemoveTags parses: REMOVE TAGS 'tag1', 'tag2' from DATASET 'dataset_name';
func (p *Parser) parseRemoveTags() (*Command, error) {
	p.nextToken() // consume TAGS

	// Parse first tag
	tag, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected tag: %w", err)
	}
	tags := []string{tag}

	// Parse additional tags separated by commas
	for {
		p.nextToken()
		if p.curToken.Type == TokenComma {
			p.nextToken()
			tag, err := p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected tag after comma: %w", err)
			}
			tags = append(tags, tag)
		} else {
			break
		}
	}
	cmd := NewCommand("rm_tags")
	cmd.Params["tags"] = tags

	// Expect from
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM after tags")
	}
	p.nextToken()

	// Expect DATASET
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after FROM")
	}
	p.nextToken()

	// Parse dataset_name
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_name: %w", err)
	}
	cmd.Params["dataset_name"] = datasetName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseRemoveChunk parses:
//   - REMOVE CHUNKS 'chunk_id1', 'chunk_id2' FROM DOCUMENT 'doc_id' IN DATASET 'dataset_name';
//   - REMOVE ALL CHUNKS FROM DOCUMENT 'doc_id' IN DATASET 'dataset_name';
func (p *Parser) parseRemoveChunk() (*Command, error) {
	cmd := NewCommand("remove_chunks")

	// Check if ALL CHUNKS - if we came here from TokenAll case, curToken is already ALL
	if p.curToken.Type == TokenAll {
		p.nextToken() // consume ALL
		if p.curToken.Type != TokenChunks {
			return nil, fmt.Errorf("expected CHUNKS after ALL")
		}
		p.nextToken() // consume CHUNKS
		cmd.Params["delete_all"] = true
	} else {
		// curToken is TokenChunks, consume it first
		p.nextToken()
		// Multiple chunks: REMOVE CHUNKS 'id1' 'id2' FROM DOCUMENT 'doc_id' IN DATASET 'dataset_name' (space-separated)
		// Parse first chunk ID
		chunkID, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected chunk_id: %w", err)
		}
		chunkIDs := []string{chunkID}

		// Parse additional chunk IDs separated by spaces (each quoted)
		for {
			p.nextToken()
			// Stop if we hit FROM or non-quoted token
			if p.curToken.Type == TokenFrom || p.curToken.Type != TokenQuotedString {
				break
			}
			chunkID, err := p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected chunk_id: %w", err)
			}
			chunkIDs = append(chunkIDs, chunkID)
		}
		cmd.Params["chunk_ids"] = chunkIDs
	}

	// Expect FROM
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM after chunk(s)")
	}
	p.nextToken()

	// Expect DOCUMENT
	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after FROM")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd.Params["doc_id"] = docID

	p.nextToken()

	// Expect IN
	if p.curToken.Type != TokenIn {
		return nil, fmt.Errorf("expected IN after doc_id")
	}
	p.nextToken()

	// Expect DATASET
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after IN")
	}
	p.nextToken()

	// Parse dataset_name (quoted string)
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_name: %w", err)
	}
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseShowTask parses SHOW ADMIN SERVER
func (p *Parser) parseUserShowAdmin() (*Command, error) {
	p.nextToken() // consume ADMIN

	var cmd *Command
	switch p.curToken.Type {
	case TokenServer:
		p.nextToken()
		cmd = NewCommand("show_admin_server")
	default:
		return nil, fmt.Errorf("expected SERVER after ADMIN")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseShowTask parses SHOW API SERVER <server_name>
func (p *Parser) parseUserShowAPI() (*Command, error) {
	p.nextToken() // consume API

	var cmd *Command
	switch p.curToken.Type {
	case TokenServer:
		p.nextToken()
		cmd = NewCommand("show_api_server")

		serverName, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected dataset_name: %w", err)
		}
		cmd.Params["api_server_name"] = serverName
		p.nextToken()

	default:
		return nil, fmt.Errorf("expected SERVER after API")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseListApiCommand() (*Command, error) {
	p.nextToken() // consume API

	var cmd *Command
	switch p.curToken.Type {
	case TokenServer:
		p.nextToken()

		cmd = NewCommand("list_api_server")
		p.nextToken()

	default:
		return nil, fmt.Errorf("expected SERVER after API")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseExplainCommand() (*Command, error) {
	p.nextToken() // consume EXPLAIN

	switch p.curToken.Type {
	case TokenChunk:
		return p.parseChunkCommand(true)
	default:
		return nil, fmt.Errorf("expected CHUNK after EXPLAIN")
	}
}

func (p *Parser) parseChunkCommand(explain bool) (*Command, error) {
	p.nextToken() // consume CHUNK

	filename, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected filename: %w", err)
	}
	p.nextToken()

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after filename")
	}
	p.nextToken()

	dsl, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected DSL: %w", err)
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	p.nextToken()

	cmd := NewCommand("user_chunk_command")
	cmd.Params["dsl"] = dsl
	cmd.Params["filename"] = filename
	cmd.Params["explain"] = explain

	return cmd, nil
}
