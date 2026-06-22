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
	"ragflow/internal/common"
	"strings"
)

// region AUTH commands
func (p *Parser) parseAdminLoginUser() (*Command, error) {
	cmd := NewCommand("login_user")

	p.nextToken() // consume LOGIN
	if p.curToken.Type != TokenAdmin {
		return nil, fmt.Errorf("expected ADMIN after LOGIN")
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

func (p *Parser) parseAdminLogout() (*Command, error) {
	cmd := NewCommand("logout")
	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminPingServer() (*Command, error) {
	cmd := NewCommand("ping_server")
	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// endregion

// region LIST commands
func (p *Parser) parseAdminListCommands() (*Command, error) {
	p.nextToken() // consume LIST

	switch p.curToken.Type {
	case TokenServices:
		return p.parseAdminListServices()
	case TokenUsers:
		return p.parseAdminListUsersCommand()
	case TokenRoles:
		return p.parseAdminListRoles()
	case TokenResources:
		return p.parseAdminListResources()
	case TokenVars:
		return p.parseAdminListVariables()
	case TokenConfigs:
		return p.parseAdminListConfigs()
	case TokenEnvs:
		return p.parseAdminListEnvironments()
	case TokenAvailable:
		return p.parseListAvailableProviders()
	case TokenProvider:
		return p.parseAdminListProviderModels()
	case TokenProviders:
		return p.parseAdminListProviders()
	case TokenModels:
		return p.parseAdminListModels()
	case TokenUser:
		return p.parseAdminListUserCommand()
	case TokenIngestors:
		return p.parseAdminListIngestors()
	case TokenIngestion:
		return p.parseAdminListIngestionTasks()
	case TokenAPI:
		return p.parseListApiCommand()
	default:
		return nil, fmt.Errorf("unknown LIST target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminListServices() (*Command, error) {
	p.nextToken() // consume SERVICES

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("admin_list_services"), nil
}

func (p *Parser) parseAdminListRoles() (*Command, error) {
	p.nextToken() // consume ROLES

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("admin_list_roles_command"), nil
}

func (p *Parser) parseAdminListResources() (*Command, error) {
	p.nextToken() // consume RESOURCES

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("admin_list_resources_command"), nil
}

func (p *Parser) parseAdminListVariables() (*Command, error) {
	p.nextToken() // consume VARIABLES

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("admin_list_variables"), nil
}

func (p *Parser) parseAdminListConfigs() (*Command, error) {
	p.nextToken() // consume CONFIGS

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("admin_list_configs"), nil
}

func (p *Parser) parseAdminListEnvironments() (*Command, error) {
	p.nextToken() // consume ENVIRONMENTS

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("admin_list_environments"), nil
}

func (p *Parser) parseListAvailableProviders() (*Command, error) {
	p.nextToken() // consume AVAILABLE

	if p.curToken.Type != TokenProviders {
		return nil, fmt.Errorf("expected PROVIDERS")
	}

	return NewCommand("admin_list_available_providers"), nil
}

func (p *Parser) parseAdminListFiles() (*Command, error) {
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
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminListIngestors() (*Command, error) {
	p.nextToken() // consume TASKS
	cmd := NewCommand("admin_list_ingestors")

	return cmd, nil
}

func (p *Parser) parseAdminListIngestionTasks() (*Command, error) {
	p.nextToken() // consume Ingestion

	if p.curToken.Type != TokenTasks {
		return nil, fmt.Errorf("expected TASKS")
	}
	p.nextToken() // consume TASKS

	cmd := NewCommand("list_admin_ingestion_tasks")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// LIST PROVIDER 'provider_name' MODELS;
func (p *Parser) parseAdminListProviderModels() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS")
	}
	p.nextToken() // consume MODELS
	cmd := NewCommand("admin_list_provider_models")
	cmd.Params["provider_name"] = providerName

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseAdminListProviders parses LIST PROVIDERS command
func (p *Parser) parseAdminListProviders() (*Command, error) {
	p.nextToken() // consume PROVIDERS
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("admin_list_providers"), nil
}

func (p *Parser) parseAdminListModels() (*Command, error) {
	p.nextToken() // consume MODELS
	cmd := NewCommand("admin_list_all_models")

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// endregion LIST commands

// region SHOW commands

func (p *Parser) parseAdminShowCommands() (*Command, error) {
	p.nextToken() // consume SHOW

	switch p.curToken.Type {
	case TokenService:
		return p.parseAdminShowService()
	case TokenUser:
		return p.parseAdminShowUserCommands()
	case TokenRole:
		return p.parseAdminShowRole()
	case TokenVersion:
		return p.parseAdminShowVersion()
	case TokenVar:
		return p.parseAdminShowVariable()
	case TokenCurrent:
		return p.parseAdminShowCurrent()
	case TokenFingerprint:
		return p.parseAdminShowFingerprint()
	case TokenLicense:
		return p.parseAdminShowLicense()
	case TokenProvider:
		return p.parseAdminShowProvider()
	case TokenModel:
		return p.parseAdminShowModel()
	case TokenAdmin:
		return p.parseUserShowAdmin()
	case TokenAPI:
		return p.parseUserShowAPI()
	case TokenUsers:
		return p.parseAdminShowUsersCommands()
	case TokenData:
		return p.parseAdminShowData()
	case TokenQuota:
		return p.parseAdminShowQuota()
	case TokenTasks:
		return p.parseAdminShowTasks()
	default:
		return nil, fmt.Errorf("unknown SHOW target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminShowService() (*Command, error) {
	p.nextToken() // consume SERVICE
	serviceIndex, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_show_service")
	cmd.Params["service_index"] = serviceIndex

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW USER 'user@example.com';
// SHOW USER 'user@example.com' ACTIVITY;
// SHOW USER 'user@example.com' SUMMARY;
// SHOW USER 'user@example.com' DATASET 'dataset_name';
// SHOW USER 'user@example.com' STORAGE;
// SHOW USER 'user@example.com' QUOTA;
// SHOW USER 'user@example.com' INDEX;
// SHOW USER 'user@example.com' PERMISSION;
func (p *Parser) parseAdminShowUserCommands() (*Command, error) {
	p.nextToken() // consume USER

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	switch p.curToken.Type {
	case TokenActivity:
		return p.parseAdminShowActivityCommand(userName)
	case TokenSummary:
		return p.parseAdminShowUserSummaryCommand(userName)
	case TokenDataset:
		return p.parseAdminShowUserDataSetCommand(userName)
	case TokenStorage:
		return p.parseAdminShowUserStorageCommand(userName)
	case TokenQuota:
		return p.parseAdminShowUserQuotaCommand(userName)
	case TokenIndex:
		return p.parseAdminShowUserIndexCommand(userName)
	case TokenPermission:
		return p.parseAdminShowUserPermissionCommand(userName)
	default:
		return p.parseAdminShowUser(userName)
	}
}

// SHOW USER 'user@example.com';
func (p *Parser) parseAdminShowUser(userName string) (*Command, error) {

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("admin_show_user")
	cmd.Params["user_name"] = userName

	return cmd, nil
}

// SHOW USER 'user@example.com' ACTIVITY DAYS 30;
func (p *Parser) parseAdminShowActivityCommand(userName string) (*Command, error) {
	p.nextToken() // consume ACTIVITY

	var days int
	var err error

	if p.curToken.Type == TokenDays {
		p.nextToken() // consume DAYS
		days, err = p.parseNumber()
		if err != nil {
			return nil, err
		}
		if days < 1 {
			return nil, fmt.Errorf("invalid number of DAYS")
		}
		p.nextToken()
	} else {
		days = 7
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("admin_show_user_activity_command")
	cmd.Params["user_name"] = userName
	cmd.Params["days"] = days

	return cmd, nil
}

// SHOW USER 'user@example.com' SUMMARY;
func (p *Parser) parseAdminShowUserSummaryCommand(userName string) (*Command, error) {
	p.nextToken() // consume SUMMARY

	cmd := NewCommand("admin_show_user_summary_command")
	cmd.Params["user_name"] = userName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// SHOW USER 'user@example.com' DATASET 'dataset_name';
func (p *Parser) parseAdminShowUserDataSetCommand(userName string) (*Command, error) {
	p.nextToken() // consume DATASET

	var tree = false
	var datasetName string
	var err error
	datasetName, err = p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type == TokenTree {
		tree = true
		p.nextToken()
	}

	cmd := NewCommand("admin_show_user_dataset_command")
	cmd.Params["user_name"] = userName
	if datasetName != "" {
		cmd.Params["dataset_name"] = datasetName
	}
	if tree {
		cmd.Params["tree"] = true
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// SHOW USER 'user@example.com' STORAGE;
func (p *Parser) parseAdminShowUserStorageCommand(userName string) (*Command, error) {
	p.nextToken() // consume STORAGE

	cmd := NewCommand("admin_show_user_storage_command")
	cmd.Params["user_name"] = userName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// SHOW USER 'user@example.com' QUOTA;
func (p *Parser) parseAdminShowUserQuotaCommand(userName string) (*Command, error) {
	p.nextToken() // consume QUOTA

	cmd := NewCommand("admin_show_user_quota_command")
	cmd.Params["user_name"] = userName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// SHOW USER 'user@example.com' INDEX;
func (p *Parser) parseAdminShowUserIndexCommand(userName string) (*Command, error) {
	p.nextToken() // consume INDEX

	cmd := NewCommand("admin_show_user_index_command")
	cmd.Params["user_name"] = userName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// SHOW USER 'user@example.com' PERMISSION;
func (p *Parser) parseAdminShowUserPermissionCommand(userName string) (*Command, error) {
	p.nextToken() // consume PERMISSION

	cmd := NewCommand("admin_show_user_permission_command")
	cmd.Params["user_name"] = userName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW ROLE 'role_name';
// SHOW ROLE 'role_name' DEFAULT MODELS;
func (p *Parser) parseAdminShowRole() (*Command, error) {
	p.nextToken() // consume ROLE

	roleName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	var cmd *Command

	switch p.curToken.Type {
	case TokenPermission:
		p.nextToken()
		cmd = NewCommand("admin_show_role_permission")
		cmd.Params["role_name"] = roleName
	case TokenDefault:
		p.nextToken()
		if p.curToken.Type != TokenModels {
			return nil, fmt.Errorf("expect MODELS after DEFAULT")
		}
		p.nextToken()
		cmd = NewCommand("admin_show_role_default_models")
		cmd.Params["role_name"] = roleName
	case TokenSemicolon:
		p.nextToken()
		cmd = NewCommand("admin_show_role")
		cmd.Params["role_name"] = roleName
	default:
		return nil, fmt.Errorf("invalid command %s", tokenTypeToString(p.curToken.Type))
	}

	return cmd, nil
}

// SHOW VERSION;
func (p *Parser) parseAdminShowVersion() (*Command, error) {
	p.nextToken() // consume VERSION

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("admin_show_version_command"), nil
}

// SHOW VAR 'var_name';
func (p *Parser) parseAdminShowVariable() (*Command, error) {
	p.nextToken() // consume VAR
	varName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_show_variable")
	cmd.Params["var_name"] = varName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW CURRENT;
func (p *Parser) parseAdminShowCurrent() (*Command, error) {
	p.nextToken() // consume CURRENT

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("admin_show_current"), nil
}

// SHOW FINGERPRINT;
func (p *Parser) parseAdminShowFingerprint() (*Command, error) {
	p.nextToken() // consume FINGERPRINT

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("admin_show_fingerprint"), nil
}

// SHOW LICENSE;
func (p *Parser) parseAdminShowLicense() (*Command, error) {
	p.nextToken() // consume LICENSE

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("admin_show_license"), nil
}

// SHOW PROVIDER 'provider_name';
func (p *Parser) parseAdminShowProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}
	p.nextToken()

	if p.curToken.Type == TokenModel {
		// SHOW PROVIDER 'provider_name' MODEL 'model_name'
		return p.parseAdminShowProviderModel(providerName)
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("admin_show_provider")
	cmd.Params["provider_name"] = providerName
	return cmd, nil
}

// SHOW PROVIDER 'provider_name' MODEL 'model_name';
func (p *Parser) parseAdminShowProviderModel(providerName string) (*Command, error) {
	p.nextToken() // consume MODEL

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}
	p.nextToken() // consume model_name

	cmd := NewCommand("admin_show_provider_model")
	cmd.Params["model_name"] = modelName
	cmd.Params["provider_name"] = providerName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW MODEL 'model_name';
func (p *Parser) parseAdminShowModel() (*Command, error) {
	p.nextToken() // consume MODEL

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("admin_show_model")
	cmd.Params["model_name"] = modelName
	return cmd, nil
}

func (p *Parser) parseCommonShowPoolModel() (*Command, error) {
	p.nextToken() // consume POOL
	if p.curToken.Type == TokenProvider {
		p.nextToken()
		providerName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd := NewCommand("show_pool_provider")
		cmd.Params["provider_name"] = providerName
		p.nextToken()
		// Semicolon is optional
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	} else if p.curToken.Type == TokenModel {
		p.nextToken() // skip model
		modelName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		p.nextToken() // skip model name
		if p.curToken.Type != TokenFrom {
			return nil, fmt.Errorf("expected FROM")
		}
		p.nextToken() // skip from
		providerName, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		p.nextToken() // skip provider name
		cmd := NewCommand("show_pool_model")
		cmd.Params["provider_name"] = providerName
		cmd.Params["model_name"] = modelName
		// Semicolon is optional
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	}

	return nil, fmt.Errorf("expected PROVIDERS or MODELS")
}

// endregion SHOW commands

// parseAdminCheck
func (p *Parser) parseAdminCheck() (*Command, error) {
	p.nextToken() // consume CHECK
	switch p.curToken.Type {
	case TokenLicense:
		return p.parseAdminCheckLicense()
	default:
		return nil, fmt.Errorf("unknown CHECK target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminCheckLicense() (*Command, error) {
	p.nextToken() // consume LICENSE
	cmd := NewCommand("admin_check_license")

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// STOP INGESTION TASKS 'task_id1 task_id2';
func (p *Parser) parseAdminStopIngestionTasks() (*Command, error) {
	p.nextToken() // consume STOP

	var cmd *Command

	switch p.curToken.Type {
	case TokenIngestion:
		p.nextToken()
		if p.curToken.Type != TokenTasks {
			return nil, fmt.Errorf("expected TASKS")
		}
		p.nextToken() // consume TASK

		taskString, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}

		tasks := strings.Split(taskString, " ")
		p.nextToken() // consume TASK

		cmd = NewCommand("admin_stop_ingestion_tasks")
		cmd.Params["tasks"] = tasks
	case TokenUser:
		return p.parseAdminStopUserCommand()
	default:
		return nil, fmt.Errorf("expected USER or INGESTION")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseAdminRemoveIngestionTasks() (*Command, error) {
	p.nextToken() // consume INGESTION

	if p.curToken.Type != TokenTasks {
		return nil, fmt.Errorf("expected TASKS")
	}
	p.nextToken() // consume TASKS

	taskString, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	tasks := strings.Split(taskString, " ")
	p.nextToken() // consume TASKS

	cmd := NewCommand("admin_remove_ingestion_tasks")
	cmd.Params["tasks"] = tasks

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// region CREATE commands
func (p *Parser) parseAdminCreateCommand() (*Command, error) {
	p.nextToken() // consume CREATE

	switch p.curToken.Type {
	case TokenUser:
		return p.parseAdminCreateUser()
	case TokenRole:
		return p.parseAdminCreateRole()
	default:
		return nil, fmt.Errorf("unknown CREATE target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminCreateUser() (*Command, error) {
	p.nextToken() // consume USER
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	var cmd *Command
	switch p.curToken.Type {
	case TokenQuotedString:
		var password string
		password, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		p.nextToken()

		cmd = NewCommand("admin_create_user")
		cmd.Params["user_name"] = userName
		cmd.Params["password"] = password
		cmd.Params["role"] = "user"
	case TokenKey:
		return p.parseAdminCreateUserAPIKeyCommand(userName)
	default:
		return nil, fmt.Errorf("expected password or KEY after USER, got %s", p.curToken.Value)
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// CREATE USER 'user@example.com' KEY;
func (p *Parser) parseAdminCreateUserAPIKeyCommand(userName string) (*Command, error) {
	p.nextToken() // consume KEY

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("admin_create_user_api_key")
	cmd.Params["user_name"] = userName

	return cmd, nil
}

func (p *Parser) parseAdminCreateRole() (*Command, error) {
	p.nextToken() // consume ROLE
	roleName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_create_role")
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if p.curToken.Type == TokenDescription {
		p.nextToken()
		var description string
		description, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["description"] = description
		p.nextToken()
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// endregion CREATE commands

// region DROP commands
func (p *Parser) parseAdminDropCommands() (*Command, error) {
	p.nextToken() // consume DROP

	switch p.curToken.Type {
	case TokenUser:
		return p.parseAdminDropUser()
	case TokenRole:
		return p.parseAdminDropRole()
	default:
		return nil, fmt.Errorf("unknown DROP target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminDropUser() (*Command, error) {
	p.nextToken() // consume USER

	if p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expected USER name, got %s", p.curToken.Value)
	}

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	switch p.curToken.Type {
	case TokenKey:
		return p.parseAdminDropUserAPIKey(userName)
	default:
		p.nextToken()
	}

	cmd := NewCommand("admin_drop_user")
	cmd.Params["user_name"] = userName
	return cmd, nil
}

func (p *Parser) parseAdminDropUserAPIKey(userName string) (*Command, error) {
	p.nextToken() // consume KEY

	apiKey, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("admin_drop_user_api_key")
	cmd.Params["user_name"] = userName
	cmd.Params["api_key"] = apiKey

	return cmd, nil
}

func (p *Parser) parseAdminDropRole() (*Command, error) {
	p.nextToken() // consume ROLE
	roleName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_drop_role")
	cmd.Params["role_name"] = roleName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminDropDataset() (*Command, error) {
	p.nextToken() // consume DATASET
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_user_dataset")
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminDropChat() (*Command, error) {
	p.nextToken() // consume CHAT
	chatName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("drop_user_chat")
	cmd.Params["chat_name"] = chatName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// endregion DROP commands

// region ALTER commands
func (p *Parser) parseAdminAlterCommands() (*Command, error) {
	p.nextToken() // consume ALTER

	switch p.curToken.Type {
	case TokenUser:
		return p.parseAdminAlterUser()
	case TokenRole:
		return p.parseAdminAlterRole()
	default:
		return nil, fmt.Errorf("unknown ALTER target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminAlterUser() (*Command, error) {
	p.nextToken() // consume USER

	if p.curToken.Type == TokenActive {
		return p.parseAdminActivateUser()
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

		cmd := NewCommand("admin_alter_user")
		cmd.Params["user_name"] = userName
		cmd.Params["password"] = password

		p.nextToken()
		// Semicolon is optional
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
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminActivateUser() (*Command, error) {
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
	p.nextToken()

	cmd := NewCommand("admin_activate_user")
	cmd.Params["user_name"] = userName
	cmd.Params["activate_status"] = status

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminAlterRole() (*Command, error) {
	p.nextToken() // consume ROLE

	roleName, err := p.parseQuotedString()
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
	p.nextToken()

	cmd := NewCommand("admin_alter_role")
	cmd.Params["role_name"] = roleName
	cmd.Params["description"] = description

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// endregion ALTER commands

func (p *Parser) parseAdminGrantCommands() (*Command, error) {
	p.nextToken() // consume GRANT

	if p.curToken.Type == TokenAdmin {
		return p.parseAdminGrantAdmin()
	}

	return p.parseAdminGrantPermission()
}

func (p *Parser) parseAdminGrantAdmin() (*Command, error) {
	p.nextToken() // consume ADMIN
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_grant_user_admin")
	cmd.Params["user_name"] = userName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminGrantPermission() (*Command, error) {
	actionListStr, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	actions := strings.Split(actionListStr, ",")
	p.nextToken()
	for idx, _ := range actions {
		actions[idx] = strings.TrimSpace(actions[idx])
	}

	if p.curToken.Type != TokenOn {
		return nil, fmt.Errorf("expected ON")
	}
	p.nextToken()

	resource, err := p.parseQuotedString()
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

	roleName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_grant_role_permission")
	cmd.Params["actions"] = actions
	cmd.Params["resource"] = resource
	cmd.Params["role_name"] = roleName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminRevokeCommands() (*Command, error) {
	p.nextToken() // consume REVOKE

	if p.curToken.Type == TokenAdmin {
		return p.parseAdminRevokeAdmin()
	}

	return p.parseAdminRevokePermission()
}

func (p *Parser) parseAdminRevokeAdmin() (*Command, error) {
	p.nextToken() // consume ADMIN
	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_revoke_user_admin")
	cmd.Params["user_name"] = userName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminRevokePermission() (*Command, error) {
	actionListStr, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	actions := strings.Split(actionListStr, ",")
	p.nextToken()
	for idx, _ := range actions {
		actions[idx] = strings.TrimSpace(actions[idx])
	}

	if p.curToken.Type != TokenOn {
		return nil, fmt.Errorf("expected ON")
	}
	p.nextToken()

	resource, err := p.parseQuotedString()
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

	roleName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_revoke_role_permission")
	cmd.Params["actions"] = actions
	cmd.Params["resource"] = resource
	cmd.Params["role_name"] = roleName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminIdentifierList() ([]string, error) {
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

func (p *Parser) parseAdminSetCommand() (*Command, error) {
	p.nextToken() // consume SET

	switch p.curToken.Type {
	case TokenLicense:
		return p.parseAdminSetLicense()
	case TokenVar:
		return p.parseAdminSetVariable()
	case TokenRole:
		return p.parseAdminSetRoleDefaultModel()
	default:
		return nil, fmt.Errorf("unknown SET target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminSetLicense() (*Command, error) {
	p.nextToken() // consume LICENSE

	if p.curToken.Type == TokenConfig {
		p.nextToken() // consume CONFIG
		// SET LICENSE CONFIG <number1> <number2>
		cmd := NewCommand("admin_set_license_config_command")
		number1, err := p.parseNumber()
		if err != nil {
			return nil, err
		}
		p.nextToken()
		number2, err := p.parseNumber()
		if err != nil {
			return nil, err
		}
		p.nextToken()
		cmd.Params["number1"] = number1
		cmd.Params["number2"] = number2
		return cmd, nil
	}

	license, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	cmd := NewCommand("admin_set_license_command")
	cmd.Params["license"] = license

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminSetVariable() (*Command, error) {
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

	cmd := NewCommand("set_variable")
	cmd.Params["var_name"] = varName
	cmd.Params["var_value"] = varValue

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminSetRoleDefaultModel() (*Command, error) {
	p.nextToken() // consume ROLE

	roleName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

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
	p.nextToken()

	modelNameOrID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_set_role_default_model")
	cmd.Params["role_name"] = roleName
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

func (p *Parser) parseAdminResetCommand() (*Command, error) {
	p.nextToken() // consume RESET

	if p.curToken.Type != TokenRole {
		return nil, fmt.Errorf("expected ROLE")
	}
	p.nextToken()

	roleName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

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
	p.nextToken()

	cmd := NewCommand("admin_reset_role_default_model")
	cmd.Params["role_name"] = roleName
	cmd.Params["model_type"] = modelType

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminBenchmarkCommand() (*Command, error) {
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

func (p *Parser) parseAdminUserStatement() (*Command, error) {
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
	case TokenRetrieve:
		return p.parseRetrieveCommand()
	default:
		return nil, fmt.Errorf("invalid user statement: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminStartService() (*Command, error) {
	p.nextToken() // consume START

	if p.curToken.Type != TokenService {
		return nil, fmt.Errorf("expected SERVICE")
	}
	p.nextToken()

	serviceIndex, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_start_service")
	cmd.Params["service_index"] = serviceIndex

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminShutdownCommands() (*Command, error) {
	p.nextToken() // consume SHUTDOWN

	switch p.curToken.Type {
	case TokenService:
		return p.parseAdminShutdownService()
	case TokenIngestor:
		return p.parseAdminShutdownIngestor()
	default:
		return nil, fmt.Errorf("expected SERVICE or INGESTOR")
	}
}

func (p *Parser) parseAdminShutdownService() (*Command, error) {
	p.nextToken() // consume SERVICE

	serviceIndex, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_shutdown_service")
	cmd.Params["service_index"] = serviceIndex

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminShutdownIngestor() (*Command, error) {
	p.nextToken() // consume INGESTOR

	ingestorName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_shutdown_ingestor_command")
	cmd.Params["ingestor_name"] = ingestorName

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminRestart() (*Command, error) {
	p.nextToken() // consume RESTART
	if p.curToken.Type != TokenService {
		return nil, fmt.Errorf("expected SERVICE")
	}
	p.nextToken()

	serviceIndex, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_restart_service")
	cmd.Params["service_index"] = serviceIndex

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminAddCommand() (*Command, error) {
	p.nextToken() // consume ADD
	switch p.curToken.Type {
	case TokenAPI:
		return p.parseAddAPIServer()
	case TokenAdmin:
		return p.parseAddAdminServer()
	case TokenProvider:
		return p.parseAdminAddModelProvider()
	default:
		return nil, fmt.Errorf("unknown ADD target: %s", p.curToken.Value)
	}
}

// ADD PROVIDER <name>
// ADD PROVIDER <name> INSTANCE <name>
func (p *Parser) parseAdminAddModelProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}
	p.nextToken()

	if p.curToken.Type == TokenInstance {
		return p.parseAdminAddModelInstance(providerName)
	}

	cmd := NewCommand("admin_add_provider")
	cmd.Params["provider_name"] = providerName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminAddModelInstance(providerName string) (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model instance name: %w", err)
	}
	p.nextToken()

	if p.curToken.Type == TokenModel {
		return p.parseAdminAddModel(providerName, instanceName)
	}

	cmd := NewCommand("admin_add_model_instance")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminAddModel(providerName, instanceName string) (*Command, error) {
	p.nextToken() // consume MODEL

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}
	p.nextToken()

	cmd := NewCommand("admin_add_models")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	cmd.Params["model_names"] = []string{modelName}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminDeleteCommands() (*Command, error) {
	p.nextToken() // consume DELETE
	switch p.curToken.Type {
	case TokenAPI:
		return p.parseDeleteAPIServer()
	case TokenAdmin:
		return p.parseDeleteAdminServer()
	case TokenProvider:
		return p.parseAdminDeleteProvider()
	default:
		return nil, fmt.Errorf("unknown ADD target: %s", p.curToken.Value)
	}
}

// DELETE PROVIDER <name> command
// DELETE PROVIDER <name> INSTANCE <name> command
// DELETE PROVIDER <name> INSTANCE <name> MODEL <name>
func (p *Parser) parseAdminDeleteProvider() (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected provider name: %w", err)
	}
	p.nextToken()

	if p.curToken.Type == TokenInstance {
		return p.parseAdminDeleteModelInstance(providerName)
	}

	cmd := NewCommand("admin_delete_model_providers")
	cmd.Params["provider_names"] = []string{providerName}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// DELETE PROVIDER <name> INSTANCE <name> command
// DELETE PROVIDER <name> INSTANCE <name> MODEL <name>
func (p *Parser) parseAdminDeleteModelInstance(providerName string) (*Command, error) {
	p.nextToken() // consume INSTANCE

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model instance name: %w", err)
	}
	p.nextToken()

	if p.curToken.Type == TokenModel {
		return p.parseAdminDeleteModel(providerName, instanceName)
	}

	cmd := NewCommand("admin_delete_model_instance")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_names"] = []string{instanceName}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// DELETE PROVIDER <name> INSTANCE <name> MODEL <name>
func (p *Parser) parseAdminDeleteModel(providerName, instanceName string) (*Command, error) {
	p.nextToken() // consume MODEL

	modelName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected model name: %w", err)
	}
	p.nextToken()

	cmd := NewCommand("admin_delete_model")
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName
	cmd.Params["model_names"] = []string{modelName}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminSaveCommand() (*Command, error) {
	p.nextToken() // consume SAVE
	switch p.curToken.Type {
	case TokenConfig:
		return p.parseSaveConfig()
	default:
		return nil, fmt.Errorf("unknown ADD target: %s", p.curToken.Value)
	}
}

func (p *Parser) parseAdminUseCommand() (*Command, error) {
	p.nextToken() // consume USE
	switch p.curToken.Type {
	case TokenAPI:
		return p.parseUseAPIServer()
	case TokenAdmin:
		return p.parseUseAdminServer()
	default:
		return nil, fmt.Errorf("expected MODEL or SKILL after USE")
	}
}

func (p *Parser) parseStartIngestion() (*Command, error) {
	p.nextToken() // consume Start

	if p.curToken.Type != TokenIngestion {
		return nil, fmt.Errorf("expect INGESTION")
	}
	p.nextToken() // consume Ingest

	uri, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_start_ingestion_command")
	cmd.Params["uri"] = uri
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseStopIngestion() (*Command, error) {
	p.nextToken() // consume Stop

	if p.curToken.Type != TokenIngestion {
		return nil, fmt.Errorf("expect INGESTION")
	}
	p.nextToken() // consume Ingest

	taskID, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_stop_ingestion_command")
	cmd.Params["task_id"] = taskID
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseAdminIngestCommand() (*Command, error) {
	p.nextToken() // consume Ingest

	uri, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("admin_ingest_command")
	cmd.Params["uri"] = uri
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}
func (p *Parser) parseAdminUnsetCommand() (*Command, error) {
	p.nextToken() // consume UNSET

	if p.curToken.Type != TokenToken {
		return nil, fmt.Errorf("expected TOKEN after UNSET")
	}
	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return NewCommand("unset_token"), nil
}

func (p *Parser) parseMessageQueueCommand() (*Command, error) {
	p.nextToken() // consume MESSAGE_QUEUE

	var cmd *Command
	switch p.curToken.Type {
	case TokenShow:
		p.nextToken()
		cmd = NewCommand("user_show_message_queue_command")

	case TokenList:
		p.nextToken() // consume LIST

		cmd = NewCommand("user_list_message_queue_command")
		if p.curToken.Type == TokenPending {
			cmd.Params["pending"] = true
			p.nextToken() // consume PENDING
		} else {
			cmd.Params["pending"] = false
		}
	case TokenPublish:
		p.nextToken() // consume PUBLISH

		message, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected message after PUBLISH")
		}
		p.nextToken() // consume message

		cmd = NewCommand("user_publish_message_command")
		cmd.Params["message"] = message
	case TokenPull:
		p.nextToken() // consume PULL

		messageCount, err := p.parseNumber()
		if err != nil {
			messageCount = 1
		} else {
			p.nextToken() // consume NUMBER
		}

		if messageCount <= 0 || messageCount > 100 {
			return nil, fmt.Errorf("message count cannot be less than 0 or greater than 100")
		}

		cmd = NewCommand("user_pull_message_command")
		cmd.Params["message_count"] = messageCount

		if p.curToken.Type == TokenNoACK {
			cmd.Params["ack_policy"] = "NOACK"
			p.nextToken() // consume NOACK
		} else {
			cmd.Params["ack_policy"] = "ACK"
		}

	default:
		return nil, fmt.Errorf("expected WITH")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// REMOVE INGESTION TASK 'task_id';
// REMOVE USER 'user@example.com' INGESTION TASKS 'created';
func (p *Parser) parseAdminRemoveCommands() (*Command, error) {
	p.nextToken() // consume REMOVE

	switch p.curToken.Type {
	case TokenIngestion:
		return p.parseAdminRemoveIngestionTasks()
	case TokenUser:
		return p.parseAdminRemoveUserCommand()
	default:
		return nil, fmt.Errorf("expected SERVICE")
	}
}

// SHOW USERS SUMMARY;
// SHOW USERS ACTIVITY;
func (p *Parser) parseAdminShowUsersCommands() (*Command, error) {
	p.nextToken() // consume USERS

	switch p.curToken.Type {
	case TokenSummary:
		p.nextToken()
		cmd := NewCommand("admin_show_users_summary_command")
		return cmd, nil
	case TokenActivity:
		return p.parseAdminShowUsersActivity()
	default:
		return nil, fmt.Errorf("invalid command")
	}
}

// SHOW USERS ACTIVITY WINDOW 2 DAYS 30;
func (p *Parser) parseAdminShowUsersActivity() (*Command, error) {
	p.nextToken() // consume ACTIVITY

	var days int
	var err error
	var windowSize int

commandLoop:
	for {
		switch p.curToken.Type {
		case TokenDays:
			p.nextToken()
			days, err = p.parseNumber()
			if err != nil {
				return nil, err
			}
			if days < 1 {
				return nil, fmt.Errorf("invalid number of DAYS")
			}
			p.nextToken()
		case TokenWindow:
			p.nextToken()
			windowSize, err = p.parseNumber()
			if err != nil {
				return nil, err
			}
			if windowSize < 0 {
				return nil, fmt.Errorf("invalid number of WINDOWS")
			}
			p.nextToken()
		case TokenSemicolon:
			p.nextToken()
			break commandLoop // done
		default:
			// No more options to process
			break commandLoop
		}
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("admin_show_users_activity_command")
	cmd.Params["days"] = days
	cmd.Params["window"] = windowSize
	return cmd, nil
}

// LIST USERS;
// LIST USERS ACTIVE 30 DAYS; // default 7 days
// LIST USERS INACTIVE 30 DAYS; // default 7 days
// LIST USERS STORAGE TOP 10;
// LIST USERS DOCUMENTS TOP 10;
// LIST USERS INDEX TOP 10;
// LIST USERS QUOTA TOP 10;
// LIST USERS QUOTA OVER;
// LIST USERS PLAN 'plan_name' QUOTA OVER DAYS 30; // default 7 days
// LIST USERS PLAN 'plan_name' DAYS 30;            // default 7 days
func (p *Parser) parseAdminListUsersCommand() (*Command, error) {
	p.nextToken() // consume USERS

	var orderBy string
	var userStatus string
	var top *int
	var plan *string
	var quota *int
	var days *int
	condition := false
commandLoop:
	for {
		switch p.curToken.Type {
		case TokenTop:
			condition = true
			p.nextToken()
			topInt, err := p.parseNumber()
			if err != nil {
				return nil, err
			}
			if topInt < 0 {
				return nil, fmt.Errorf("invalid number of TOP")
			}
			p.nextToken()
			top = &topInt
		case TokenDays:
			condition = true
			p.nextToken()
			daysInt, err := p.parseNumber()
			if err != nil {
				return nil, err
			}
			if daysInt < 0 {
				return nil, fmt.Errorf("invalid number of DAYS")
			}
			p.nextToken()
			days = &daysInt
		case TokenPlan:
			condition = true
			p.nextToken()
			planStr, err := p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			if planStr == "" {
				return nil, fmt.Errorf("invalid plan")
			}
			plan = &planStr
			p.nextToken()
		case TokenQuota:
			condition = true
			p.nextToken()
			quotaInt, err := p.parseNumber()
			if err != nil {
				return nil, err
			}
			if quotaInt < 0 {
				return nil, fmt.Errorf("invalid number of QUOTA")
			}
			quota = &quotaInt
			p.nextToken()
		case TokenDocuments, TokenIndex, TokenStorage:
			condition = true
			if orderBy != "" {
				return nil, fmt.Errorf("order by already set")
			}
			orderBy = p.curToken.Value
			p.nextToken()
		case TokenActive, TokenInactive:
			condition = true
			userStatus = p.curToken.Value
			p.nextToken()
		case TokenSemicolon:
			p.nextToken()
			break commandLoop // done
		default:
			// No more options to process
			break commandLoop
		}
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	if !condition {
		return NewCommand("admin_list_users_command"), nil
	}

	cmd := NewCommand("admin_list_users_condition_command")
	if orderBy != "" {
		cmd.Params["order_by"] = orderBy
	}
	if userStatus != "" {
		cmd.Params["user_status"] = userStatus
	}
	if top != nil {
		cmd.Params["top"] = *top
	}
	if plan != nil {
		cmd.Params["plan"] = *plan
	}
	if quota != nil {
		cmd.Params["quota"] = *quota
	}
	if days != nil {
		cmd.Params["days"] = *days
	}

	return cmd, nil
}

// SHOW DATA SUMMARY;
// SHOW DATA ORPHAN;
// SHOW DATA STORAGE;
// SHOW DATA INDEX;
func (p *Parser) parseAdminShowData() (*Command, error) {
	p.nextToken() // consume DATA

	var cmd *Command
	switch p.curToken.Type {
	case TokenSummary:
		p.nextToken()
		cmd = NewCommand("admin_show_data_summary")
	case TokenOrphan:
		p.nextToken()
		cmd = NewCommand("admin_show_data_orphan")
	case TokenStorage:
		p.nextToken()
		cmd = NewCommand("admin_show_data_storage")
	case TokenIndex:
		p.nextToken()
		cmd = NewCommand("admin_show_data_index")
	default:
		return nil, fmt.Errorf("expected SUMMARY, ORPHAN, STORAGE, INDEX after DATA")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW QUOTA SUMMARY;
func (p *Parser) parseAdminShowQuota() (*Command, error) {
	p.nextToken() // consume QUOTA

	var cmd *Command
	switch p.curToken.Type {
	case TokenSummary:
		p.nextToken()
		cmd = NewCommand("admin_show_quota_summary")
	default:
		return nil, fmt.Errorf("expected SUMMARY after QUOTA")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// SHOW TASKS SUMMARY;
func (p *Parser) parseAdminShowTasks() (*Command, error) {
	p.nextToken() // consume TASKS

	var cmd *Command
	switch p.curToken.Type {
	case TokenSummary:
		p.nextToken()
		cmd = NewCommand("admin_show_tasks_summary")
	default:
		return nil, fmt.Errorf("expected SUMMARY after TASKS")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// PURGE PREVIEW ORPHAN
// PURGE ORPHAN

// PURGE PREVIEW USER 'user@example.com';
// PURGE USER

// PURGE PREVIEW USERS PLAN 'plan_name' DAYS 30; // default 7 days
// PURGE USERS PLAN 'plan_name' DAYS 30;

// PURGE PREVIEW USERS INACTIVE PLAN 'plan_name' DAYS 30;
// PURGE USERS INACTIVE PLAN 'plan_name' DAYS 30;
func (p *Parser) parseAdminPurgeCommand() (*Command, error) {
	p.nextToken() // consume PURGE
	var preview = false
	if p.curToken.Type == TokenPreview {
		p.nextToken()
		preview = true
	}

	switch p.curToken.Type {
	case TokenOrphan:
		return p.parseAdminPurgeOrphanCommand(preview)
	case TokenUser:
		return p.parseAdminPurgeUserCommand(preview)
	case TokenUsers:
		return p.parseAdminPurgeUsersCommand(preview)
	default:
		return nil, fmt.Errorf("expected PREVIEW, USER, USERS after PURGE")
	}
}

// PURGE PREVIEW ORPHAN
// PURGE ORPHAN
func (p *Parser) parseAdminPurgeOrphanCommand(preview bool) (*Command, error) {
	p.nextToken() // consume ORPHAN

	cmd := NewCommand("admin_purge_orphan_command")
	cmd.Params["preview"] = preview
	return cmd, nil
}

// PURGE PREVIEW USER 'user@example.com';
// PURGE USER 'user@example.com';
func (p *Parser) parseAdminPurgeUserCommand(preview bool) (*Command, error) {
	p.nextToken() // consume USER

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	cmd := NewCommand("admin_purge_user_command")
	cmd.Params["preview"] = preview
	cmd.Params["user_name"] = userName
	return cmd, nil
}

// PURGE PREVIEW USERS PLAN 'plan_name' DAYS 30; // default 7 days
// PURGE USERS PLAN 'plan_name' DAYS 30;
// PURGE PREVIEW USERS INACTIVE PLAN 'plan_name' DAYS 30;
// PURGE USERS INACTIVE PLAN 'plan_name' DAYS 30;
func (p *Parser) parseAdminPurgeUsersCommand(preview bool) (*Command, error) {
	p.nextToken() // consume USERS

	var userStatus *string = nil
	var days *int = nil
	var planName *string = nil

commandLoop:
	for {
		switch p.curToken.Type {
		case TokenPlan:
			p.nextToken()
			if planName != nil {
				return nil, fmt.Errorf("duplicate PLAN after USERS")
			}
			plan, err := p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			planName = &plan
			p.nextToken()
		case TokenDays:
			p.nextToken()
			if days != nil {
				return nil, fmt.Errorf("duplicate DAYS after USERS")
			}
			dayCount, err := p.parseNumber()
			if err != nil {
				return nil, err
			}
			days = &dayCount
			p.nextToken()
		case TokenInactive:
			p.nextToken()
			if userStatus != nil {
				return nil, fmt.Errorf("duplicate INACTIVE or ACTIVE after USERS")
			}
			inactiveStatus := "inactive"
			userStatus = &inactiveStatus
		case TokenActive:
			p.nextToken()
			if userStatus != nil {
				return nil, fmt.Errorf("duplicate INACTIVE or ACTIVE after USERS")
			}
			activeStatus := "active"
			userStatus = &activeStatus
		case TokenSemicolon:
			p.nextToken()
			break commandLoop // done
		default:
			// No more options to process
			break commandLoop
		}
	}

	cmd := NewCommand("admin_purge_users_command")
	cmd.Params["preview"] = preview
	if planName != nil {
		cmd.Params["plan_name"] = *planName
	}
	if userStatus != nil {
		cmd.Params["user_status"] = *userStatus
	}
	if days != nil {
		cmd.Params["days"] = *days
	}
	return cmd, nil
}

// LIST USER 'user@example.com' INGESTION TASKS;
// LIST USER 'user@example.com' DATASETS;
// LIST USER 'user@example.com' AGENTS;
// LIST USER 'user@example.com' CHATS;
// LIST USER 'user@example.com' SEARCHES;
// LIST USER 'user@example.com' MODELS; // all added models
// LIST USER 'user@example.com' FILES;
// LIST USER 'user@example.com' KEYS;
// LIST USER 'user_name' PROVIDER 'provider_name' INSTANCE 'instance_name' MODELS;
func (p *Parser) parseAdminListUserCommand() (*Command, error) {
	p.nextToken() // consume USER

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	var cmd *Command

	switch p.curToken.Type {
	case TokenIngestion:
		return p.parseAdminListUserIngestionTasks(userName)
	case TokenDatasets:
		p.nextToken()
		cmd = NewCommand("admin_list_user_datasets")
	case TokenAgents:
		p.nextToken()
		cmd = NewCommand("admin_list_user_agents")
	case TokenChats:
		p.nextToken()
		cmd = NewCommand("admin_list_user_chats")
	case TokenSearches:
		p.nextToken()
		cmd = NewCommand("admin_list_user_searches")
	case TokenModels:
		p.nextToken()
		cmd = NewCommand("admin_list_user_models")
	case TokenFiles:
		p.nextToken()
		cmd = NewCommand("admin_list_user_files")
	case TokenKeys:
		p.nextToken()
		cmd = NewCommand("admin_list_user_keys")
	case TokenProvider:
		return p.parseAdminListUserProviderInstanceModels(userName)
	case TokenProviders:
		p.nextToken()
		cmd = NewCommand("admin_list_user_providers")
	case TokenDefault:
		return p.parseAdminListUserDefaultModels(userName)
	default:
		return nil, fmt.Errorf("expected INGESTION or DATASETS or AGENTS or CHATS or SEARCHES or MODELS or FILES or KEYS after USER")
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd.Params["user_name"] = userName
	return cmd, nil
}

// LIST USER 'user@example.com' INGESTION TASKS 'status';
func (p *Parser) parseAdminListUserIngestionTasks(userName string) (*Command, error) {
	p.nextToken() // consume INGESTION

	if p.curToken.Type != TokenTasks {
		return nil, fmt.Errorf("expected TASKS after INGESTION")
	}
	p.nextToken()

	cmd := NewCommand("admin_list_user_ingestion_tasks")
	cmd.Params["user_name"] = userName

	if p.curToken.Type == TokenQuotedString {
		var status string
		var err error
		status, err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["status"] = status
		p.nextToken()
	}
	return cmd, nil
}

// LIST USER 'user_name' PROVIDER 'provider_name' INSTANCES;
// LIST USER 'user_name' PROVIDER 'provider_name' INSTANCE 'instance_name' MODELS;
func (p *Parser) parseAdminListUserProviderInstanceModels(userName string) (*Command, error) {
	p.nextToken() // consume PROVIDER

	providerName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type == TokenInstances {
		p.nextToken()
		cmd := NewCommand("admin_list_user_provider_instances")
		cmd.Params["user_name"] = userName
		cmd.Params["provider_name"] = providerName

		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
		}
		return cmd, nil
	}

	if p.curToken.Type != TokenInstance {
		return nil, fmt.Errorf("expected INSTANCE after PROVIDER")
	}
	p.nextToken()

	instanceName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS after INSTANCE")
	}
	p.nextToken()

	cmd := NewCommand("admin_list_user_provider_instance_models")
	cmd.Params["user_name"] = userName
	cmd.Params["provider_name"] = providerName
	cmd.Params["instance_name"] = instanceName

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// LIST USER 'user_name' DEFAULT MODELS;
func (p *Parser) parseAdminListUserDefaultModels(userName string) (*Command, error) {
	p.nextToken() // consume DEFAULT

	if p.curToken.Type != TokenModels {
		return nil, fmt.Errorf("expected MODELS after INSTANCE")
	}
	p.nextToken()

	cmd := NewCommand("admin_list_user_default_models")
	cmd.Params["user_name"] = userName

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// STOP USER 'user@example.com' INGESTION TASKS 'created';
func (p *Parser) parseAdminStopUserCommand() (*Command, error) {
	p.nextToken() // consume USER

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	var cmd *Command
	switch p.curToken.Type {
	case TokenIngestion:
		p.nextToken()
		if p.curToken.Type != TokenTasks {
			return nil, fmt.Errorf("expected TASKS after INGESTION")
		}
		p.nextToken()
		cmd = NewCommand("admin_stop_user_ingestion_tasks_command")
		if p.curToken.Type == TokenQuotedString {
			var status string
			status, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["status"] = status
			p.nextToken()
		}
	default:
		return nil, fmt.Errorf("expected INGESTION after USER")
	}

	cmd.Params["user_name"] = userName
	return cmd, nil
}

// REMOVE USER 'user@example.com' INGESTION TASKS 'created';
func (p *Parser) parseAdminRemoveUserCommand() (*Command, error) {
	p.nextToken() // consume USER

	userName, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}
	p.nextToken()

	var cmd *Command
	switch p.curToken.Type {
	case TokenIngestion:
		p.nextToken()
		if p.curToken.Type != TokenTasks {
			return nil, fmt.Errorf("expected TASKS after INGESTION")
		}
		p.nextToken()
		cmd = NewCommand("admin_remove_user_ingestion_tasks_command")
		if p.curToken.Type == TokenQuotedString {
			var status string
			status, err = p.parseQuotedString()
			if err != nil {
				return nil, err
			}
			cmd.Params["status"] = status
			p.nextToken()
		}
	default:
		return nil, fmt.Errorf("expected INGESTION after USER")
	}

	cmd.Params["user_name"] = userName
	return cmd, nil
}
