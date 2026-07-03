package cli

import (
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"regexp"
	"strings"
)

func tokenTypeDescription(t int, tok Token) string {
	if tok.Type == t && tok.Value != "" {
		return fmt.Sprintf("%s %q", tokenTypeToString(t), tok.Value)
	}
	return tokenTypeToString(t)
}

// Command parsers
func (p *Parser) parseAPILoginUser() (*Command, error) {
	cmd := NewCommand("api_login_user")

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
		var password string
		password, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["password"] = password
		p.nextToken()
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseAPILogout() (*Command, error) {
	cmd := NewCommand("api_logout")
	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIPingServer() (*Command, error) {
	cmd := NewCommand("api_ping_server")
	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIRegister() (*Command, error) {
	cmd := NewCommand("api_register_user")

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

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// region LIST commands

// LIST CONFIGS;
// LIST PROVIDER 'provider_name' MODELS;
// LIST PROVIDER 'provider_name' INSTANCE 'instance_name' MODELS
// LIST MODELS;
func (p *Parser) parseAPIListCommands() (*Command, error) {
	p.nextToken() // consume LIST

	switch p.curToken.Type {
	case TokenConfigs:
		return p.parseAPIListConfigs()
	case TokenDatasets:
		return p.parseAPIListDatasets()
	case TokenDataset:
		return p.parseAPIListDatasetCommands()
	case TokenAgents:
		return p.parseAPIListAgents()
	case TokenChats:
		return p.parseAPIListChats()
	case TokenSearches:
		return p.parseAPIListSearches()
	case TokenMemories:
		return p.parseAPIListMemories()
	case TokenKeys:
		return p.parseAPIListAPIKeys()
	case TokenProviders:
		return p.parseAPIListProviders()
	case TokenProvider:
		return p.parseAPIListProviderCommands()
	case TokenModels:
		return p.parseAPIListAllModels()
	case TokenIngestion:
		return p.parseAPIListIngestionTasks()
	case TokenDefault:
		return p.parseAPIListDefaultModels()
	case TokenAvailable:
		return p.parseAPIListAvailableProviders()
	case TokenAPI:
		return p.parseAPIListAPIServers()
	case TokenEnvs:
		return p.parseAPIListEnvironments()
	case TokenVars:
		return p.parseAPIListVariables()
	default:
		return nil, fmt.Errorf("unknown LIST target: %s", p.curToken.Value)
	}
}

// LIST CONFIGS;
func (p *Parser) parseAPIListConfigs() (*Command, error) {
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("api_list_configs"), nil
}

func (p *Parser) parseAPIListDatasets() (*Command, error) {
	cmd := NewCommand("api_list_datasets")
	p.nextToken() // consume DATASETS

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// LIST DATASET 'dataset_name' DOCUMENTS;
func (p *Parser) parseAPIListDatasetCommands() (*Command, error) {
	p.nextToken() // consume DATASET

	datasetID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	switch p.curToken.Type {
	case TokenDocuments:
		return p.parseAPIListDatasetDocuments(datasetID)
	case TokenFiles:
		return p.parseAPIListDatasetFiles(datasetID)
	case TokenIngestion:
		return p.parseAPIListDatasetIngestionTasks(datasetID)
	default:
		return nil, fmt.Errorf("unknown LIST target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAPIListDatasetDocuments(datasetID string) (*Command, error) {
	p.nextToken() // consume DOCUMENTS

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_list_dataset_documents")
	cmd.Params["dataset_id"] = datasetID
	return cmd, nil
}

func (p *Parser) parseAPIListDatasetFiles(datasetName string) (*Command, error) {
	p.nextToken() // consume FILES

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_list_dataset_files")
	cmd.Params["dataset_name"] = datasetName
	return cmd, nil
}

func (p *Parser) parseAPIListDatasetIngestionTasks(datasetName string) (*Command, error) {
	p.nextToken() // consume INGESTION

	if p.curToken.Type != TokenTasks {
		return nil, fmt.Errorf("expected TASKS")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_list_ingestion_tasks")
	cmd.Params["dataset_name"] = datasetName
	return cmd, nil
}

func (p *Parser) parseAPIListAgents() (*Command, error) {
	p.nextToken() // consume AGENTS

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("api_list_agents"), nil
}

func (p *Parser) parseAPIListChats() (*Command, error) {
	p.nextToken() // consume CHATS

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("api_list_chats"), nil
}

func (p *Parser) parseAPIListSearches() (*Command, error) {
	p.nextToken() // consume SEARCHES

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("api_list_searches"), nil
}

func (p *Parser) parseAPIListMemories() (*Command, error) {
	p.nextToken() // consume MEMORIES

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("api_list_memories"), nil
}

func (p *Parser) parseAPIListAPIKeys() (*Command, error) {
	p.nextToken() // consume KEYS
	cmd := NewCommand("api_list_api_keys")
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// LIST PROVIDERS
func (p *Parser) parseAPIListProviders() (*Command, error) {
	p.nextToken() // consume PROVIDERS
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("api_list_providers"), nil
}

// LIST PROVIDER 'provider_name' INSTANCES
func (p *Parser) parseAPIListProviderCommands() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	switch p.curToken.Type {
	case TokenInstances:
		return p.parseListProviderInstances(providerName)
	case TokenInstance:
		return p.parseListProviderInstanceCommands(providerName)
	case TokenModels:
		return p.parseListProviderModels(providerName)
	default:
		return nil, fmt.Errorf("unknown LIST target: %s", p.curToken.Value)
	}
}

// LIST PROVIDER 'provider_name' INSTANCES
func (p *Parser) parseListProviderInstances(providerName string) (*Command, error) {
	p.nextToken() // consume INSTANCES

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	cmd := NewCommand("api_list_provider_instances")
	cmd.Params["provider_name"] = providerName
	return cmd, nil
}

// LIST PROVIDER 'provider_name' INSTANCE 'instance_name' MODELS
// LIST PROVIDER 'provider_name' INSTANCE 'instance_name' MODELS SYNC, get model list by API from remote server
func (p *Parser) parseListProviderInstanceCommands(providerName string) (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	switch p.curToken.Type {
	case TokenModels:
		return p.parseListProviderInstanceModels(providerName, instanceName)
	case TokenTasks:
		return p.parseListProviderInstanceTasks(providerName, instanceName)
	default:
		return nil, fmt.Errorf("unknown LIST target: %s", p.curToken.Value)
	}

}

func (p *Parser) parseListProviderInstanceModels(providerName, instanceName string) (*Command, error) {
	p.nextToken() // consume MODELS

	cmd := NewCommand("api_list_provider_instance_models")
	if p.curToken.Type == TokenSync {
		cmd = NewCommand("api_list_provider_instance_models_sync")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	return cmd, nil
}

func (p *Parser) parseListProviderInstanceTasks(providerName, instanceName string) (*Command, error) {
	p.nextToken() // consume TASKS

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_list_provider_instance_tasks")

	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	return cmd, nil
}

func (p *Parser) parseListProviderModels(providerName string) (*Command, error) {
	p.nextToken() // consume MODELS

	cmd := NewCommand("api_list_provider_models")
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	cmd.Params["provider_name"] = providerName
	return cmd, nil
}

// LIST MODELS
func (p *Parser) parseAPIListAllModels() (*Command, error) {
	p.nextToken() // consume models

	cmd := NewCommand("api_list_all_models")

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIListIngestionTasks() (*Command, error) {
	p.nextToken() // consume Ingestion

	if p.curToken.Type != TokenTasks {
		return nil, fmt.Errorf("expected TASKS")
	}
	p.nextToken() // consume TASKS

	cmd := NewCommand("api_list_ingestion_tasks")

	if p.curToken.Type == TokenFrom {
		p.nextToken()
		datasetID, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["dataset_id"] = datasetID
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIListDefaultModels() (*Command, error) {
	p.nextToken() // consume DEFAULT
	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS")
	}
	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("api_list_default_models"), nil
}

func (p *Parser) parseAPIListAvailableProviders() (*Command, error) {
	p.nextToken() // consume AVAILABLE

	if p.curToken.Type != TokenProviders {
		return nil, fmt.Errorf("expected PROVIDERS")
	}

	return NewCommand("api_list_available_providers"), nil
}

func (p *Parser) parseAPIListAPIServers() (*Command, error) {
	p.nextToken() // consume API

	var cmd *Command
	switch p.curToken.Type {
	case TokenServer:
		p.nextToken()
		cmd = NewCommand("api_list_api_servers")
	default:
		return nil, fmt.Errorf("expected SERVER after API")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseAPIListEnvironments() (*Command, error) {
	p.nextToken() // consume Envs

	cmd := NewCommand("api_list_environments")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseAPIListVariables() (*Command, error) {
	p.nextToken() // consume Variables

	cmd := NewCommand("api_list_variables")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// endregion LIST commands

// region SHOW commands

func (p *Parser) parseAPIShowCommands() (*Command, error) {
	p.nextToken() // consume SHOW
	switch p.curToken.Type {
	case TokenVersion:
		return p.parseAPIShowVersion()
	case TokenKey:
		return p.parseAPIShowKey()
	case TokenCurrent:
		return p.parseAPIShowCurrent()
	case TokenVar:
		return p.parseAPIShowVariable()
	case TokenProvider:
		return p.parseAPIShowProviderCommands()
	case TokenModel:
		return p.parseAPIShowModel()
	case TokenAdmin:
		return p.parseAPIShowAdmin()
	case TokenAPI:
		return p.parseAPIShowAPI()
	case TokenLog:
		return p.parseAPIShowLogCommands()
	default:
		return nil, fmt.Errorf("unknown SHOW target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAPIShowVersion() (*Command, error) {
	p.nextToken() // consume VERSION

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("api_show_version"), nil
}

func (p *Parser) parseAPIShowKey() (*Command, error) {
	p.nextToken() // consume KEY

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("api_show_api_key"), nil
}

func (p *Parser) parseAPIShowCurrent() (*Command, error) {
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("api_show_current"), nil
}

func (p *Parser) parseAPIShowVariable() (*Command, error) {
	p.nextToken() // consume VAR
	varName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_show_variable")
	cmd.Params["var_name"] = varName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW PROVIDER <name>;
// SHOW PROVIDER <name> INSTANCE <instance_name>;
// SHOW PROVIDER <name> INSTANCE <instance_name> BALANCE;
// SHOW PROVIDER <name> INSTANCE <instance_name> TASK <task_id>;
// SHOW PROVIDER 'provider_name' MODEL 'model_name';
func (p *Parser) parseAPIShowProviderCommands() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}
	p.nextToken() // consume provider_name

	switch p.curToken.Type {
	case TokenInstance:
		return p.parseAPIShowProviderInstance(providerName)
	case TokenModel:
		return p.parseAPIShowProviderModel(providerName)
	case TokenSemicolon, TokenEOF:
		p.nextToken()
	default:
		return nil, fmt.Errorf("unknown SHOW target: %s", p.curToken.Value)
	}

	cmd := NewCommand("api_show_provider")
	cmd.Params["provider_name"] = providerName

	return cmd, nil
}

// SHOW PROVIDER <name> INSTANCE <instance_name>;
func (p *Parser) parseAPIShowProviderInstance(providerName string) (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name: %w", err)
	}
	p.nextToken() // consume instance_name

	switch p.curToken.Type {
	case TokenBalance:
		return p.parseAPIShowProviderInstanceBalance(providerName, instanceName)
	case TokenTask:
		return p.parseAPIShowProviderInstanceTask(providerName, instanceName)
	case TokenSemicolon, TokenEOF:
		p.nextToken()
	default:
		return nil, fmt.Errorf("unknown SHOW target: %s", p.curToken.Value)
	}

	cmd := NewCommand("api_show_provider_instance")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["provider_name"] = providerName

	return cmd, nil
}

// SHOW PROVIDER <name> INSTANCE <instance_name> BALANCE
func (p *Parser) parseAPIShowProviderInstanceBalance(providerName, instanceName string) (*Command, error) {
	p.nextToken() // consume BALANCE

	cmd := NewCommand("api_show_provider_instance_balance")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["provider_name"] = providerName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW PROVIDER <name> INSTANCE <instance_name> TASK <task_id>
func (p *Parser) parseAPIShowProviderInstanceTask(providerName, instanceName string) (*Command, error) {
	p.nextToken() // consume TASK

	taskID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected task id: %w", err)
	}
	p.nextToken() // consume task_id

	cmd := NewCommand("api_show_provider_instance_task")
	cmd.Params["instance_name"] = instanceName
	cmd.Params["provider_name"] = providerName
	cmd.Params["task_id"] = taskID

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW PROVIDER <name> MODEL <model_name>
func (p *Parser) parseAPIShowProviderModel(providerName string) (*Command, error) {
	p.nextToken() // consume MODEL

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}
	p.nextToken() // consume model_name

	cmd := NewCommand("api_show_provider_model")
	cmd.Params["model_name"] = modelName
	cmd.Params["provider_name"] = providerName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW MODEL 'model_name';
func (p *Parser) parseAPIShowModel() (*Command, error) {
	p.nextToken() // consume MODEL

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}
	p.nextToken() // consume model_name

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	cmd := NewCommand("api_show_model")
	cmd.Params["model_name"] = modelName
	return cmd, nil
}

// SHOW ADMIN SERVER
func (p *Parser) parseAPIShowAdmin() (*Command, error) {
	p.nextToken() // consume ADMIN

	var cmd *Command
	switch p.curToken.Type {
	case TokenServer:
		p.nextToken()
		cmd = NewCommand("api_show_admin_server")
	default:
		return nil, fmt.Errorf("expected SERVER after ADMIN")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW API SERVER <server_name>
func (p *Parser) parseAPIShowAPI() (*Command, error) {
	p.nextToken() // consume API

	var cmd *Command
	switch p.curToken.Type {
	case TokenServer:
		p.nextToken()
		cmd = NewCommand("api_show_api_server")

		serverName, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected API server name: %w", err)
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

func (p *Parser) parseAPIShowLogCommands() (*Command, error) {
	p.nextToken() // consume LOG

	switch p.curToken.Type {
	case TokenLevel:
		return p.parseShowLogLevel()
	default:
		return nil, fmt.Errorf("expected LEVEL after LOG")
	}
}

func (p *Parser) parseShowLogLevel() (*Command, error) {
	p.nextToken() // consume LEVEL

	cmd := NewCommand("api_show_log_level")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// endregion SHOW commands

// region CREATE commands

func (p *Parser) parseAPICreateCommands() (*Command, error) {
	p.nextToken() // consume CREATE

	switch p.curToken.Type {
	case TokenDataset:
		return p.parseAPICreateDataset()
	case TokenChat:
		return p.parseAPICreateChat()
	case TokenSearch:
		return p.parseAPICreateSearch()
	case TokenAgent:
		return p.parseAPICreateAgent()
	case TokenMemory:
		return p.parseAPICreateMemory()
	case TokenKey:
		return p.parseAPICreateKey()
	case TokenChunkStore:
		return p.parseDevCreateChunkStore()
	case TokenMetadata:
		return p.parseDevCreateMetadataStore()
	case TokenProvider:
		return p.parseAPICreateProviderInstance()
	default:
		return nil, fmt.Errorf("unknown CREATE target: %s", p.curToken.Value)
	}
}

// CREATE DATASET 'abc';
func (p *Parser) parseAPICreateDataset() (*Command, error) {
	p.nextToken() // consume DATASET
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_create_dataset")
	cmd.Params["dataset_name"] = datasetName
	return cmd, nil
}

// CREATE CHAT 'chat_name'
func (p *Parser) parseAPICreateChat() (*Command, error) {
	p.nextToken() // consume CHAT
	chatName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_create_chat")
	cmd.Params["chat_name"] = chatName

	return cmd, nil
}

// CREATE SEARCH 'search_name'
func (p *Parser) parseAPICreateSearch() (*Command, error) {
	p.nextToken() // consume SEARCH
	searchName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_create_search")
	cmd.Params["search_name"] = searchName

	return cmd, nil
}

// CREATE AGENT 'agent_name'
func (p *Parser) parseAPICreateAgent() (*Command, error) {
	p.nextToken() // consume AGENT

	agentName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_create_agent")
	cmd.Params["agent_name"] = agentName
	return cmd, nil
}

// CREATE MEMORY 'memory_name'
func (p *Parser) parseAPICreateMemory() (*Command, error) {
	p.nextToken() // consume MEMORY
	memoryName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_create_memory")
	cmd.Params["memory_name"] = memoryName

	return cmd, nil
}

// CREATE KEY 'key_name'
func (p *Parser) parseAPICreateKey() (*Command, error) {
	p.nextToken() // consume KEY

	// Semicolon is optional for UNSET KEY
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("api_create_api_key"), nil
}

// CREATE PROVIDER <name> INSTANCE <instance_name> KEY <api_key> URL <base_url> REGION <region>
func (p *Parser) parseAPICreateProviderInstance() (*Command, error) {
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

	cmd := NewCommand("api_create_provider_instance")
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

// endregion CREATE commands

// region DROP commands
func (p *Parser) parseAPIDropCommands() (*Command, error) {
	p.nextToken() // consume DROP

	switch p.curToken.Type {
	case TokenDataset:
		return p.parseAPIDropDataset()
	case TokenChat:
		return p.parseAPIDropChat()
	case TokenSearch:
		return p.parseAPIDropSearch()
	case TokenMemory:
		return p.parseAPIDropMemory()
	case TokenAgent:
		return p.parseAPIDropAgent()
	case TokenKey:
		return p.parseAPIDropAPIKey()
	case TokenChunkStore:
		return p.parseDevDropChunkStore()
	case TokenMetadata:
		return p.parseDevDropMetadataStore()
	default:
		return nil, fmt.Errorf("unknown DROP target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAPIDropDataset() (*Command, error) {
	p.nextToken() // consume DATASET
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_drop_dataset")
	cmd.Params["dataset_name"] = datasetName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIDropChat() (*Command, error) {
	p.nextToken() // consume CHAT
	chatName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("api_drop_chat")
	cmd.Params["chat_name"] = chatName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIDropSearch() (*Command, error) {
	p.nextToken() // consume SEARCH
	searchName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_drop_search")
	cmd.Params["search_name"] = searchName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIDropMemory() (*Command, error) {
	p.nextToken() // consume MEMORY
	memoryName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("api_drop_memory")
	cmd.Params["memory_name"] = memoryName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIDropAgent() (*Command, error) {
	p.nextToken() // consume AGENT
	agentName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("api_drop_agent")
	cmd.Params["agent_name"] = agentName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIDropAPIKey() (*Command, error) {
	p.nextToken() // consume KEY

	apiKey, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_delete_api_key")
	cmd.Params["api_key"] = apiKey

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// endregion DROP commands

// region ADD commands

func (p *Parser) parseAPIAddCommands() (*Command, error) {
	p.nextToken() // consume ADD
	switch p.curToken.Type {
	case TokenProvider:
		return p.parseAPIAddProvider()
	case TokenModel:
		return p.parseAPIAddModel()
	case TokenAPI:
		return p.parseAddAPIServer()
	case TokenAdmin:
		return p.parseAddAdminServer()
	default:
		return nil, fmt.Errorf("unknown ADD target: %s", p.curToken.Value)
	}
}

// parseAPIAddProvider parses ADD PROVIDER commands
// ADD PROVIDER <name>
// ADD PROVIDER <name> INSTANCE <instance> KEY <key> [URL <url> | REGION <region>]
func (p *Parser) parseAPIAddProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}
	p.nextToken()

	switch p.curToken.Type {
	case TokenInstance:
		return p.parseAddProviderInstance(providerName)
	case TokenEOF, TokenSemicolon:
		p.nextToken()
	default:
		return nil, fmt.Errorf("unknown ADD PROVIDER target: %s", p.curToken.Value)
	}

	cmd := NewCommand("api_add_provider")
	cmd.Params["provider_name"] = providerName

	p.nextToken()
	return cmd, nil
}

// ADD PROVIDER <name> INSTANCE <instance>
func (p *Parser) parseAddProviderInstance(providerName string) (*Command, error) {
	p.nextToken() // consume INSTANCE

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

	cmd := NewCommand("api_add_provider_instance")
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

// ADD MODEL 'xxx' TO PROVIDER 'vllm' INSTANCE 'test' TOKENS 1024 CHAT THINK VISION;
func (p *Parser) parseAPIAddModel() (*Command, error) {
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

	modelIndex := 0
	var modelTypes []string
	var supportThink *bool = nil
	maxTokens := 0
	var maxDimension *int = nil
	var dimensions []int = nil

	models := make([]map[string]any, 0, len(modelNames))

optionsLoop:
	for {
		if modelIndex >= len(modelNames) {
			return nil, fmt.Errorf("too many model configs: got more configs than model names")
		}
		currentModelName := modelNames[modelIndex]
		switch p.curToken.Type {
		case TokenThink:
			if supportThink != nil {
				return nil, fmt.Errorf("think model is already set for model %s", currentModelName)
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
			if p.curToken.Type == TokenInteger {
				val, err := p.parseNumber()
				if err != nil {
					return nil, err
				}
				maxDimension = &val
				p.nextToken()

				dimensions = make([]int, 0)
				for p.curToken.Type == TokenInteger {
					dim, err := p.parseNumber()
					if err != nil {
						return nil, err
					}
					dimensions = append(dimensions, int(dim))
					p.nextToken()
				}
			}

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
				return nil, fmt.Errorf("max tokens is already given %d for model %s", maxTokens, currentModelName)
			}
			if p.curToken.Type != TokenInteger {
				return nil, fmt.Errorf("expected integer")
			}
			maxTokens, err = p.parseNumber()
			if err != nil {
				return nil, err
			}
			p.nextToken() // consume number

		case TokenComma, TokenSemicolon, TokenEOF:
			if len(modelTypes) == 0 {
				return nil, fmt.Errorf("model type is required for model %s", currentModelName)
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
				return nil, fmt.Errorf("model type is required for model %s", currentModelName)
			}

			model := map[string]any{
				"model_name":  currentModelName,
				"model_types": modelTypes,
				"max_tokens":  maxTokens,
			}
			if supportThink != nil {
				model["thinking"] = *supportThink
			}

			if maxDimension != nil {
				model["max_dimension"] = *maxDimension
				model["dimensions"] = dimensions
			}

			models = append(models, model)

			modelIndex++
			modelTypes = nil
			supportThink = nil
			maxTokens = 0
			maxDimension = nil
			dimensions = nil

			if p.curToken.Type == TokenComma {
				p.nextToken()
				continue
			}

			if p.curToken.Type == TokenSemicolon {
				p.nextToken()
			}
			break optionsLoop

		default:
			return nil, fmt.Errorf("unexpected token type: %s", p.curToken.Value)
		}

	}
	if len(models) != len(modelNames) {
		return nil, fmt.Errorf("model config count %d does not match model name count %d", len(models), len(modelNames))
	}

	cmd := NewCommand("api_add_custom_model")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	cmd.Params["models"] = models

	return cmd, nil
}

// syntax: ADD API 'abc' HOST '127.0.0.1:9333';
func (p *Parser) parseAddAPIServer() (*Command, error) {
	p.nextToken() // consume API

	serverName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume model name

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

	cmd := NewCommand("api_add_api_server")
	cmd.Params["server_name"] = serverName
	cmd.Params["server_ip"] = ip
	cmd.Params["server_port"] = port

	return cmd, nil
}

// syntax: add admin host '127.0.0.1:9333' user 'ccc' password 'ppp'
func (p *Parser) parseAddAdminServer() (*Command, error) {
	p.nextToken() // consume ADMIN

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

	cmd := NewCommand("api_add_admin_server")
	cmd.Params["server_ip"] = ip
	cmd.Params["server_port"] = port

	return cmd, nil
}

// endregion ADD commands

// region DELETE commands

func (p *Parser) parseAPIDeleteCommands() (*Command, error) {
	p.nextToken() // consume DELETE

	switch p.curToken.Type {
	case TokenProvider:
		return p.parseAPIDeleteProvider()
	case TokenMetadata:
		return p.parseDevDeleteMeta()
	case TokenAdmin:
		return p.parseAPIDeleteAdminServer()
	case TokenAPI:
		return p.parseAPIDeleteAPIServer()
	default:
		return nil, fmt.Errorf("unknown DELETE target: %s", p.curToken.Value)
	}
}

// DELETE PROVIDER <name>
// DELETE PROVIDER <name> INSTANCE <instance_name>
func (p *Parser) parseAPIDeleteProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}
	p.nextToken()

	switch p.curToken.Type {
	case TokenInstance:
		return p.parseAPIDeleteProviderInstance(providerName)
	case TokenEOF, TokenSemicolon:
		p.nextToken()
	default:
		return nil, fmt.Errorf("unknown DELETE target: %s", p.curToken.Value)
	}

	cmd := NewCommand("api_delete_provider")
	cmd.Params["provider_name"] = providerName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIDeleteProviderInstance(providerName string) (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected instance name: %w", err)
	}
	p.nextToken()

	cmd := NewCommand("api_delete_provider_instance")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName

	switch p.curToken.Type {
	case TokenModels:
		return p.parseAPIDeleteProviderInstanceModels(providerName, instanceName)
	case TokenEOF, TokenSemicolon:
		p.nextToken()
	default:
		return nil, fmt.Errorf("unknown DELETE target: %s", p.curToken.Value)
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// DELETE PROVIDER <name> INSTANCE <instance_name> MODELS <name1 name2 name3>
// Only works for local deployed model
func (p *Parser) parseAPIDeleteProviderInstanceModels(providerName, instanceName string) (*Command, error) {
	p.nextToken() // consume MODEL

	rawModels, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}
	p.nextToken()

	modelNames := strings.Split(rawModels, " ")

	cmd := NewCommand("api_delete_provider_instance_model")
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

// syntax: delete api 'abc'
func (p *Parser) parseAPIDeleteAPIServer() (*Command, error) {
	p.nextToken() // consume API

	serverName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken() // consume model name

	cmd := NewCommand("api_delete_api_server")
	cmd.Params["server_name"] = serverName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// syntax: delete admin server 'abc'
func (p *Parser) parseAPIDeleteAdminServer() (*Command, error) {
	p.nextToken() // consume ADMIN

	cmd := NewCommand("api_delete_admin_server")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// endregion ADD commands

// region ALTER commands
func (p *Parser) parseAPIAlterCommands() (*Command, error) {
	p.nextToken() // consume ALTER

	switch p.curToken.Type {
	case TokenProvider:
		return p.parseAPIAlterInstance()
	default:
		return nil, fmt.Errorf("unknown ALTER target: %s", p.curToken.Value)
	}
}

// ALTER PROVIDER <name> INSTANCE <name> [NAME <new_name> | KEY <new_key>]
func (p *Parser) parseAPIAlterInstance() (*Command, error) {
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

	newInstanceName := ""
	newAPIKey := ""
optionsLoop:
	for {
		switch p.curToken.Type {
		case TokenName:
			p.nextToken()
			newInstanceName, err = p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected model name: %w", err)
			}
			p.nextToken()
		case TokenKey:
			p.nextToken()
			newAPIKey, err = p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected API key: %w", err)
			}
			p.nextToken()
		default:
			break optionsLoop
		}
	}

	if newInstanceName == "" && newAPIKey == "" {
		return nil, fmt.Errorf("expected NAME or KEY after INSTANCE")
	}

	cmd := NewCommand("api_alter_provider_instance")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	cmd.Params["new_instance_name"] = newInstanceName
	cmd.Params["new_api_key"] = newAPIKey

	p.nextToken()
	// Semicolon is optional
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
		ident, err = p.parseIdentifier()
		if err != nil {
			return nil, err
		}
		list = append(list, ident)
		p.nextToken()
	}

	return list, nil
}

// endregion ALTER commands

// region SET commands

func (p *Parser) parseAPISetCommands() (*Command, error) {
	p.nextToken() // consume SET

	switch p.curToken.Type {
	case TokenVar:
		return p.parseAPISetVariable()
	case TokenDefault:
		return p.parseAPISetDefault()
	case TokenKey:
		return p.parseAPISetAPIKey()
	case TokenLog:
		return p.parseAPISetLog()
	case TokenMetadata:
		return p.parseDevSetMeta()
	default:
		return nil, fmt.Errorf("unknown SET target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAPISetVariable() (*Command, error) {
	p.nextToken() // consume VAR

	varName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	varValue, err := p.parseVariableValue()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_set_variable")
	cmd.Params["var_name"] = varName
	cmd.Params["var_value"] = varValue

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPISetDefault() (*Command, error) {
	p.nextToken() // consume DEFAULT

	var modelType, modelNameOrID string
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

	// Format: 'model@instance@provider' or just 'message'
	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected quoted string with format model@instance@provider")
	}

	modelNameOrID, err = p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_set_default_model")
	cmd.Params["model_type"] = modelType

	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPISetAPIKey() (*Command, error) {
	p.nextToken() // consume KEY

	apiKey, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_set_api_key")
	cmd.Params["api_key"] = apiKey

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPISetLog() (*Command, error) {
	p.nextToken() // consume LOG

	switch p.curToken.Type {
	case TokenLevel:
		return p.parseAPISetLogLevel()
	default:
		return nil, fmt.Errorf("unknown log target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAPISetLogLevel() (*Command, error) {
	p.nextToken() // consume LEVEL

	cmd := NewCommand("api_set_log_level")
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
		return nil, fmt.Errorf("unknown log level: %s", p.curToken.Value)
	}
	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// endregion SET commands

// region RESET commands

func (p *Parser) parseAPIResetCommands() (*Command, error) {
	p.nextToken() // consume RESET

	switch p.curToken.Type {
	case TokenDefault:
		return p.parseAPIResetDefaultModel()
	case TokenKey:
		return p.parseAPIResetKey()
	default:
		return nil, fmt.Errorf("unknown RESET target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAPIResetDefaultModel() (*Command, error) {
	p.nextToken() // consume DEFAULT

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

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIResetKey() (*Command, error) {
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("api_unset_api_key"), nil
}

// endregion RESET commands

func (p *Parser) parseAPIImport() (*Command, error) {
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
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIRetrieve() (*Command, error) {
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
		} else if p.curToken.Type == TokenIllegal && p.curToken.Value == "." {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = "."
			} else {
				cmd.Params["path"] = fmt.Sprintf("%s.", cmd.Params["path"])
			}
			p.nextToken()
			continue
		} else {
			return nil, fmt.Errorf("unexpected token %q in search path", p.curToken.Value)
		}
	}
	return cmd, nil
}

// region ParseCommands
func (p *Parser) parseParseCommands() (*Command, error) {
	p.nextToken() // consume PARSE

	switch p.curToken.Type {
	case TokenDataset:
		return p.parseAPIParseDataset()
	case TokenWith:
		return p.parseAPIModelParse()
	case TokenDocument:
		return p.parseAPIParseDocs()
	case TokenFile:
		return p.parseAPIParseLocalFile()
	default:
		return nil, fmt.Errorf("expected DATASET, WITH, or DOCUMENT")
	}
}

func (p *Parser) parseAPIParseDataset() (*Command, error) {
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
	p.nextToken()

	cmd := NewCommand("parse_dataset")
	cmd.Params["dataset_name"] = datasetName
	cmd.Params["method"] = method

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIModelParse() (*Command, error) {
	p.nextToken() // consume WITH

	modelNameOrID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("api_model_parse_file")

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

	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseAPIParseDocs() (*Command, error) {
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

	cmd := NewCommand("api_parse_documents")

	documents := strings.Split(documentsStr, " ")

	cmd.Params["documents"] = documents
	cmd.Params["dataset_id"] = datasetID

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIParseLocalFile() (*Command, error) {
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
			cmd.Params["embedding_model"] = embedModel
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

// endregion PARSE commands

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
		return p.parseAPIPingServer()
	case TokenShow:
		return p.parseAPIShowCommands()
	case TokenList:
		return p.parseAPIListCommands()
	case TokenImport:
		return p.parseAPIImport()
	case TokenInsert:
		return p.parseDevInsertCommand()
	case TokenRetrieve:
		return p.parseAPIRetrieve()
	default:
		return nil, fmt.Errorf("invalid user statement: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAPIEnable() (*Command, error) {
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

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("enable_model")
	cmd.Params["model_name"] = modelName
	cmd.Params["instance_name"] = modelInstance
	cmd.Params["provider_name"] = modelProvider
	return cmd, nil
}

func (p *Parser) parseAPIDisable() (*Command, error) {
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

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("disable_model")
	cmd.Params["model_name"] = modelName
	cmd.Params["instance_name"] = modelInstance
	cmd.Params["provider_name"] = modelProvider
	return cmd, nil
}

// region MODEL commands
// CHAT 'model@instance@provider' 'hello world'
// CHAT WITH 'model@instance@provider' MESSAGE 'hello world' 'who are you' IMAGE 'url1' 'file0' VIDEO "url2.mov" "file1" FILE "url" "path file2" AUDIO "file.wav"
func (p *Parser) parseAPIChat() (*Command, error) {
	p.nextToken() // consume CHAT

	// Redirect "chat completion[s]" to the standalone chat completions parser.
	if p.curToken.Type == TokenIdentifier &&
		(strings.EqualFold(p.curToken.Value, "completion") || strings.EqualFold(p.curToken.Value, "completions")) {
		p.nextToken() // consume completion/completions
		return p.parseChatCompletionsBody()
	}

	var err error
	var modelNameOrID string = ""
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
			// 'model@instance@provider' or model ID
			if modelNameOrID != "" {
				return nil, fmt.Errorf("model name or ID is already set")
			}
			modelNameOrID, err = p.parseQuotedString()
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
	cmd := NewCommand("api_chat_to_model")

	if modelNameOrID != "" {
		if common.IsCompositeModelName(modelNameOrID) {
			cmd.Params["composite_model_name"] = modelNameOrID
		} else if common.IsUUID(modelNameOrID) {
			cmd.Params["model_id"] = modelNameOrID
		} else {
			return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
		}
	}
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

func (p *Parser) parseAPIThink() (*Command, error) {

	p.nextToken() // consume THINK

	if p.curToken.Type != TokenChat {
		return nil, fmt.Errorf("expected CHAT after THINK")
	}

	command, err := p.parseAPIChat()
	if err != nil {
		return nil, err
	}
	command.Params["thinking"] = true
	return command, nil
}

func (p *Parser) parseAPIStream() (*Command, error) {

	p.nextToken() // consume STREAM

	var command *Command
	var err error

	switch p.curToken.Type {
	case TokenChat:
		command, err = p.parseAPIChat()
		if err != nil {
			return nil, err
		}
	case TokenThink:
		command, err = p.parseAPIThink()
		if err != nil {
			return nil, err
		}
	case TokenASR:
		command, err = p.parseAPIASR()
		if err != nil {
			return nil, err
		}
	case TokenTTS:
		command, err = p.parseAPITTS()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("expected CHAT, THINK, ASR, or TTS after STREAM")
	}

	command.Params["stream"] = true
	return command, nil
}

func (p *Parser) parseAPIEmbed() (*Command, error) {
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

	modelNameOrID, err := p.parseQuotedString()
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

	cmd := NewCommand("api_embed_user_text")
	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}
	cmd.Params["texts"] = texts
	if dimension > 0 {
		cmd.Params["dimension"] = dimension
	}
	return cmd, nil
}

func (p *Parser) parseAPIRerank() (*Command, error) {
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

	modelNameOrID, err := p.parseQuotedString()
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

	cmd := NewCommand("api_rarank_user_document")
	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}
	cmd.Params["query"] = query
	cmd.Params["documents"] = documents
	cmd.Params["top_n"] = topN
	return cmd, nil
}

func (p *Parser) parseAPIASR() (*Command, error) {
	p.nextToken() // consume ASR

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after ASR")
	}
	p.nextToken() // consume WITH

	modelNameOrID, err := p.parseQuotedString()
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
	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}
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

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseAPITTS() (*Command, error) {
	p.nextToken()

	cmd := NewCommand("tts_user_command")

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expect 'with' after tts")
	}
	p.nextToken()

	if p.curToken.Type != TokenQuotedString && p.curToken.Type != TokenIdentifier {
		return nil, fmt.Errorf("expect model name after 'with'")
	}

	modelNameOrID := strings.Trim(p.curToken.Value, "\"'")
	p.nextToken()

	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}

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

func (p *Parser) parseAPIOCR() (*Command, error) {
	p.nextToken() // consume OCR

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after OCR")
	}
	p.nextToken() // consume WITH

	modelNameOrID, err := p.parseQuotedString()
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

	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// endregion MODEL commands

func (p *Parser) parseAPICheck() (*Command, error) {
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

func (p *Parser) parseUserStartIngestion() (*Command, error) {
	p.nextToken() // consume Start

	if p.curToken.Type != TokenIngestion {
		return nil, fmt.Errorf("expect INGESTION")
	}
	p.nextToken() // consume Ingestion

	documentID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expect FROM")
	}
	p.nextToken() // consume FROM

	datasetID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("api_start_ingestion")
	cmd.Params["document_id"] = documentID
	cmd.Params["dataset_id"] = datasetID
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseUserStopIngestion() (*Command, error) {
	p.nextToken() // consume Stop

	if p.curToken.Type != TokenIngestion {
		return nil, fmt.Errorf("expect INGESTION")
	}
	p.nextToken() // consume Ingestion

	if p.curToken.Type != TokenTasks {
		return nil, fmt.Errorf("expect TASKS")
	}
	p.nextToken()

	taskStr, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	tasks := strings.Split(taskStr, " ")

	cmd := NewCommand("api_stop_ingestion")
	cmd.Params["tasks"] = tasks
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
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

func (p *Parser) parseAPISave() (*Command, error) {
	p.nextToken() // consume SAVE
	switch p.curToken.Type {
	case TokenConfig:
		return p.parseAPISaveConfig()
	default:
		return nil, fmt.Errorf("unknown ADD target: %s", p.curToken.Value)
	}
}

// SAVE CONFIG AS 'path'
func (p *Parser) parseAPISaveConfig() (*Command, error) {
	p.nextToken() // consume CONFIG

	if p.curToken.Type != TokenAs {
		return nil, fmt.Errorf("expected AS after CONFIG")
	}
	p.nextToken() // consume AS

	path, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("api_save_config_command")
	cmd.Params["path"] = path

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseAPIUseCommands() (*Command, error) {
	p.nextToken() // consume USE

	switch p.curToken.Type {
	case TokenModel:
		return p.parseAPIUseModel()
	case TokenAPI:
		return p.parseAPIUseAPIServer()
	case TokenAdmin:
		return p.parseAPIUseAdminServer()
	default:
		return nil, fmt.Errorf("expected MODEL or SKILL after USE")
	}
}

func (p *Parser) parseAPIUseModel() (*Command, error) {
	p.nextToken() // consume MODEL

	modelNameOrID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model identifier in format 'model@instance@provider': %w", err)
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("api_use_model")

	if common.IsCompositeModelName(modelNameOrID) {
		cmd.Params["composite_model_name"] = modelNameOrID
	} else if common.IsUUID(modelNameOrID) {
		cmd.Params["model_id"] = modelNameOrID
	} else {
		return nil, fmt.Errorf("invalid format of model name or ID: %s", modelNameOrID)
	}

	return cmd, nil
}

func (p *Parser) parseAPIUseAPIServer() (*Command, error) {
	p.nextToken() // consume API

	serverName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()
	cmd := NewCommand("api_use_api_server")
	cmd.Params["server_name"] = serverName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIUseAdminServer() (*Command, error) {
	p.nextToken() // consume ADMIN

	cmd := NewCommand("api_use_admin_server")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAPIRemove() (*Command, error) {
	p.nextToken() // consume REMOVE

	switch p.curToken.Type {
	// API commands
	case TokenIngestion:
		return p.parseAPIRemoveTask()

	// Dev commands
	case TokenTag:
		return p.parseDevRemoveTags()
	case TokenChunks, TokenAll:
		return p.parseDevRemoveChunk()
	default:
		return nil, fmt.Errorf("unknown REMOVE target: %s", p.curToken.Value)
	}
}
func (p *Parser) parseAPIRemoveTask() (*Command, error) {
	p.nextToken() // consume Ingestion

	if p.curToken.Type != TokenTasks {
		return nil, fmt.Errorf("expected TASKS")
	}
	p.nextToken() // consume TASKS

	taskStr, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("api_remove_task")

	tasks := strings.Split(taskStr, " ")

	cmd.Params["tasks"] = tasks

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseOpenaiChat parses:
//
//	OPENAI_CHAT <chat_id> <message>
//	              [model <string>] [system <string>]
//	              [history <string>] [history_delimiter <string>]
//	              [temperature <float>] [max_tokens <int>] [stream <bool>]
//	              [top_p <float>] [frequency_penalty <float>] [presence_penalty <float>]
//	              [extra_body <json>]
//	              ;
//
// Named options can appear in any order. The chat_id and message are
// required positional args; everything else is optional with a default.
//
// `history` is captured as a single string in cmd.Params["history_raw"]
// and is split into turns by cmd.Params["history_delimiter"] (default
// ";") later in buildOpenaiChatRequestBody — this two-step split lets
// `history_delimiter` and `history` appear in either order on the
// command line. The chosen delimiter must not appear inside any
// message body.
//
// `extra_body` is well-formed JSON. The accepted keys are:
//
//			reference          bool
//			reference_metadata { include?: bool, fields?: string[] }
//			metadata_condition { logic?: "and"|"or", conditions?: [{key, operator, value}] }
//	 (See user_command.go:allowedExtraBodyKeys for the authoritative set)
func (p *Parser) parseOpenaiChat() (*Command, error) {
	p.nextToken() // consume OPENAI_CHAT

	if p.curToken.Type == TokenDash {
		dashCount := 0
		for p.curToken.Type == TokenDash {
			dashCount++
			p.nextToken()
		}
		if dashCount > 0 && p.curToken.Type == TokenIdentifier {
			switch strings.ToLower(p.curToken.Value) {
			case "h", "help":
				return NewCommand("openai_chat_help"), nil
			}
		}
		return nil, fmt.Errorf("OPENAI_CHAT: only -h/--help takes no args; otherwise expected chat_id and message")
	}

	cmd := NewCommand("api_openai_chat")

	// Defaults — match the OpenAI spec / RAGFlow server behavior.
	cmd.Params["model"] = "model" // placeholder; server resolves to dialog.llm_id
	cmd.Params["temperature"] = 0.0
	cmd.Params["max_tokens"] = 0
	cmd.Params["stream"] = false

	// Required positional: <chat_id> <message>
	chatID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("OPENAI_CHAT: expected chat_id as first argument: %w", err)
	}
	cmd.Params["chat_id"] = chatID
	p.nextToken()

	message, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("OPENAI_CHAT: expected message as second argument: %w", err)
	}
	cmd.Params["message"] = message
	p.nextToken()

	// Optional
	handleOption := func(name string) error {
		switch name {
		case "model", "system":
			v, err := p.parseQuotedString()
			if err != nil {
				return fmt.Errorf("OPENAI_CHAT %s: expected quoted string, got %s", name, p.curToken.Value)
			}
			cmd.Params[name] = v
			p.nextToken()
		case "temperature", "top_p", "frequency_penalty", "presence_penalty":
			v, err := p.parseFloat()
			if err != nil {
				return fmt.Errorf("OPENAI_CHAT %s: expected number, got %s", name, p.curToken.Value)
			}
			cmd.Params[name] = v
			p.nextToken()
		case "max_tokens":
			v, err := p.parseNumber()
			if err != nil {
				return fmt.Errorf("OPENAI_CHAT max_tokens: expected integer, got %s", p.curToken.Value)
			}
			cmd.Params["max_tokens"] = v
			p.nextToken()
		case "stream":
			v, err := p.parseBool()
			if err != nil {
				return fmt.Errorf("OPENAI_CHAT %s: expected true|false, got %s", name, p.curToken.Value)
			}
			cmd.Params[name] = v
			// parseBool already advances the cursor.
		case "extra_body":
			raw, err := p.parseJSONLiteral()
			if err != nil {
				return fmt.Errorf("OPENAI_CHAT %s: %w", name, err)
			}
			cmd.Params[name] = raw
			p.nextToken()
		case "history":
			raw, err := p.parseQuotedString()
			if err != nil {
				return fmt.Errorf("OPENAI_CHAT history: expected quoted string, got %s", p.curToken.Value)
			}
			cmd.Params["history_raw"] = raw
			p.nextToken()
		case "history_delimiter":
			v, err := p.parseQuotedString()
			if err != nil {
				return fmt.Errorf("OPENAI_CHAT history_delimiter: expected quoted string, got %s", p.curToken.Value)
			}
			cmd.Params["history_delimiter"] = v
			p.nextToken()
		default:
			return fmt.Errorf("OPENAI_CHAT: unknown option %q (valid: model, system, history, history_delimiter, temperature, max_tokens, stream, top_p, frequency_penalty, presence_penalty, extra_body)", name)
		}
		return nil
	}

	// Named options, any order, until ';'.
optionsLoop:
	for {
		switch p.curToken.Type {
		case TokenSemicolon:
			p.nextToken()
			break optionsLoop
		case TokenEOF:
			break optionsLoop

		case TokenIdentifier, TokenQuotedString:
			name := p.curToken.Value
			if p.curToken.Type == TokenQuotedString {
				name = strings.Trim(name, "'\"")
			}
			p.nextToken()
			if err := handleOption(name); err != nil {
				return nil, err
			}

		default:
			if !isKeyword(p.curToken.Type) {
				return nil, fmt.Errorf("OPENAI_CHAT: unexpected token %q in option list (valid options: model, system, history, history_delimiter, temperature, max_tokens, stream, top_p, frequency_penalty, presence_penalty, extra_body)", p.curToken.Value)
			}
			name := p.curToken.Value
			p.nextToken()
			if err := handleOption(name); err != nil {
				return nil, err
			}
		}
	}

	return cmd, nil
}

//	CHAT COMPLETIONS <question>
//	                 [chat_id <string>] [session <string>] [llm <string>]

// parseChatCompletionsBody parses the question and options of a CHAT COMPLETIONS
// command. The leading keyword(s) must already have been consumed by the caller.
func (p *Parser) parseChatCompletionsBody() (*Command, error) {

	if p.curToken.Type == TokenDash {
		dashCount := 0
		for p.curToken.Type == TokenDash {
			dashCount++
			p.nextToken()
		}
		if dashCount > 0 && p.curToken.Type == TokenIdentifier {
			switch strings.ToLower(p.curToken.Value) {
			case "h", "help":
				return NewCommand("chat completions help"), nil
			}
		}
		return nil, fmt.Errorf("CHAT COMPLETIONS: only -h/--help takes no args; otherwise expected question")
	}

	cmd := NewCommand("chat completions")

	// Defaults
	cmd.Params["chat_id"] = ""
	cmd.Params["temperature"] = 0.0
	cmd.Params["max_tokens"] = 0
	cmd.Params["stream"] = false

	// Track which options were explicitly set (distinguishes from defaults).
	cmd.Params["_set"] = map[string]bool{}

	// Required positional: <question>
	question, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("CHAT COMPLETIONS: expected question: %w", err)
	}
	cmd.Params["question"] = question
	p.nextToken()

	// Optional named options
	handleOption := func(name string) error {
		switch name {
		case "chat_id", "session", "llm", "system":
			v, err := p.parseQuotedString()
			if err != nil {
				return fmt.Errorf("CHAT COMPLETIONS %s: expected quoted string, got %s", name, p.curToken.Value)
			}
			cmd.Params[name] = v
			p.nextToken()
			markSet(cmd, name)
		case "temperature", "top_p", "frequency_penalty", "presence_penalty":
			v, err := p.parseFloat()
			if err != nil {
				return fmt.Errorf("CHAT COMPLETIONS %s: expected number, got %s", name, p.curToken.Value)
			}
			cmd.Params[name] = v
			p.nextToken()
			markSet(cmd, name)
		case "max_tokens":
			v, err := p.parseNumber()
			if err != nil {
				return fmt.Errorf("CHAT COMPLETIONS max_tokens: expected integer, got %s", p.curToken.Value)
			}
			cmd.Params["max_tokens"] = v
			p.nextToken()
			markSet(cmd, "max_tokens")
		case "stream", "pass_all_history", "legacy":
			v, err := p.parseBool()
			if err != nil {
				return fmt.Errorf("CHAT COMPLETIONS %s: expected true|false, got %s", name, p.curToken.Value)
			}
			cmd.Params[name] = v
			markSet(cmd, name)
		case "history":
			raw, err := p.parseQuotedString()
			if err != nil {
				return fmt.Errorf("CHAT COMPLETIONS history: expected quoted string, got %s", p.curToken.Value)
			}
			cmd.Params["history_raw"] = raw
			p.nextToken()
		case "history_delimiter":
			v, err := p.parseQuotedString()
			if err != nil {
				return fmt.Errorf("CHAT COMPLETIONS history_delimiter: expected quoted string, got %s", p.curToken.Value)
			}
			cmd.Params["history_delimiter"] = v
			p.nextToken()
		default:
			return fmt.Errorf("CHAT COMPLETIONS: unknown option %q (valid: chat_id, session, llm, system, history, history_delimiter, temperature, max_tokens, stream, top_p, frequency_penalty, presence_penalty, pass_all_history, legacy)", name)
		}
		return nil
	}

	// Named options, any order, until ';'.
optionsLoop:
	for {
		switch p.curToken.Type {
		case TokenSemicolon:
			p.nextToken()
			break optionsLoop
		case TokenEOF:
			break optionsLoop

		case TokenIdentifier, TokenQuotedString:
			name := p.curToken.Value
			if p.curToken.Type == TokenQuotedString {
				name = strings.Trim(name, "'\"")
			}
			p.nextToken()
			if err := handleOption(name); err != nil {
				return nil, err
			}

		default:
			if !isKeyword(p.curToken.Type) {
				return nil, fmt.Errorf("CHAT COMPLETIONS: unexpected token %q in option list (valid options: chat_id, session, llm, system, history, history_delimiter, temperature, max_tokens, stream, top_p, frequency_penalty, presence_penalty, pass_all_history, legacy)", p.curToken.Value)
			}
			name := p.curToken.Value
			p.nextToken()
			if err := handleOption(name); err != nil {
				return nil, err
			}
		}
	}

	return cmd, nil
}

func markSet(cmd *Command, name string) {
	if s, ok := cmd.Params["_set"].(map[string]bool); ok {
		s[name] = true
	}
}

func isSet(cmd *Command, name string) bool {
	if s, ok := cmd.Params["_set"].(map[string]bool); ok {
		return s[name]
	}
	return false
}

// parseJSONLiteral consumes a TokenQuotedString whose payload is a JSON
// value (object, array, string, number, or boolean) and returns it as
// the original raw string (NOT decoded — the caller decides whether to
// embed it into a larger JSON object or pass it through as-is).
func (p *Parser) parseJSONLiteral() (string, error) {
	if p.curToken.Type != TokenQuotedString {
		return "", fmt.Errorf("expected JSON literal in single/double quotes, got %s", p.curToken.Value)
	}
	raw := p.curToken.Value
	// Validate it actually parses as JSON so we fail fast on
	// typos like `'{}' extra comma'` or `'not json'`.
	var probe interface{}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return "", fmt.Errorf("invalid JSON literal %q: %w", raw, err)
	}
	return raw, nil
}

// parseBool accepts a TokenIdentifier "true"/"false"
func (p *Parser) parseBool() (bool, error) {
	switch strings.ToLower(p.curToken.Value) {
	case "true":
		p.nextToken()
		return true, nil
	case "false":
		p.nextToken()
		return false, nil
	}
	return false, fmt.Errorf("expected true or false, got %q", p.curToken.Value)
}

// historyRoleRegex matches the role prefix on a turn. The captured
// alternation is the role; the colon is required so we don't
// accidentally split on a word like "user:foo" appearing inside
// other content.
var historyRoleRegex = regexp.MustCompile(`(?i)^(user|assistant):`)

// defaultHistoryDelimiter is the turn separator used when the
// caller does not pass the `history_delimiter` option.
const defaultHistoryDelimiter = ";"

// parseHistory splits the history literal into a slice of
// {"role": ..., "content": ...} maps. Format:
//
//	"user:question one;assistant:answer one;user:question two"
//
// Turns are separated by `history_delimiter` (default `;`). Each
// segment must start with the role prefix `user:` or `assistant:`
// (case-insensitive).
func parseHistory(literal, delimiter string) ([]map[string]string, error) {
	if delimiter == "" {
		delimiter = defaultHistoryDelimiter
	}

	// Trim a single pair of surrounding quotes if present.
	s := strings.TrimSpace(literal)
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = s[1 : len(s)-1]
		}
	}

	raw := strings.Split(s, delimiter)
	turns := make([]map[string]string, 0, len(raw))
	for _, segment := range raw {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		m := historyRoleRegex.FindStringSubmatch(segment)
		if m == nil {
			return nil, fmt.Errorf("history segment %q must start with 'user:' or 'assistant:'", segment)
		}
		role := strings.ToLower(m[1])
		// Drop the "<role>:" prefix (m[0] is the whole match, e.g.
		// "user:"; we want the content AFTER the colon).
		content := strings.TrimPrefix(segment, m[0])
		turns = append(turns, map[string]string{
			"role":    role,
			"content": content,
		})
	}
	if len(turns) == 0 {
		return nil, fmt.Errorf("history is empty or unparseable: %q", literal)
	}
	return turns, nil
}
