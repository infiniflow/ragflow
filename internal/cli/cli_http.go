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

import "fmt"

// ExecuteCommand executes a parsed command
// Returns benchmark result map for commands that support it (e.g., ping_server with iterations > 1)
func (c *CLI) ExecuteCommand(cmd *Command) (ResponseIf, error) {
	switch c.Config.CLIMode {
	case APIMode:
		// Interactive mode: execute command with user privileges
		return c.ExecuteUserCommand(cmd)
	case AdminMode:
		// Admin mode: execute command with admin privileges
		return c.ExecuteAdminCommand(cmd)
	default:
		return nil, fmt.Errorf("invalid server type: %s", c.Config.CLIMode)
	}
}

func (c *CLI) ExecuteAdminCommand(cmd *Command) (ResponseIf, error) {
	switch cmd.Type {
	case "login_user":
		return c.LoginUserByCommand(cmd)
	case "logout":
		return c.Logout()
	case "ping":
		return c.PingByCommand(cmd)
	case "benchmark":
		return c.RunBenchmark(cmd)
	case "list_users":
		return c.ListUsers(cmd)
	case "list_services":
		return c.ListServices(cmd)
	case "grant_admin":
		return c.GrantAdmin(cmd)
	case "revoke_admin":
		return c.RevokeAdmin(cmd)
	case "create_user":
		return c.CreateUser(cmd)
	case "activate_user":
		return c.ActivateUser(cmd)
	case "alter_user":
		return c.AlterUserPassword(cmd)
	case "drop_user":
		return c.DropUser(cmd)
	case "show_service":
		return c.ShowService(cmd)
	case "show_version":
		return c.ShowAdminVersion(cmd)
	case "show_current":
		return c.ShowCommonCurrent(cmd)
	case "show_user":
		return c.ShowUser(cmd)
	case "list_variables":
		return c.ListVariables(cmd)
	case "show_variable":
		return c.ShowVariable(cmd)
	case "set_variable":
		return c.SetVariable(cmd)
	case "list_user_datasets":
		return c.ListUserDatasets(cmd)
	case "list_agents":
		return c.ListAgents(cmd)
	case "generate_token":
		return c.GenerateAdminToken(cmd)
	case "list_tokens":
		return c.ListAdminTokens(cmd)
	case "drop_token":
		return c.DropAdminToken(cmd)
	case "list_available_providers":
		return c.ListAvailableProviders(cmd)
	case "show_provider":
		return c.ShowProvider(cmd)
	case "list_provider_models":
		return c.ListModels(cmd)
	case "list_supported_models":
		return c.ListSupportedModels(cmd)
	case "list_instance_models":
		return c.ListInstanceModels(cmd)
	case "show_provider_model":
		return c.ShowProviderModel(cmd)
	case "show_model":
		return c.ShowModel(cmd)
	case "list_all_models":
		return c.ListAllModels(cmd)
	case "list_admin_tasks":
		return c.ListAdminTasks(cmd)
	case "admin_list_ingestors":
		return c.ListAdminIngestors(cmd)
	case "admin_start_ingestion_command":
		return c.AdminStartIngestionCommand(cmd)
	case "admin_stop_ingestion_command":
		return c.AdminStopIngestionCommand(cmd)
	case "admin_shutdown_ingestor_command":
		return c.AdminShutdownIngestor(cmd)
	case "list_admin_ingestion_tasks":
		return c.ListAdminIngestionTasks(cmd)
	case "show_admin_server":
		return c.ShowAdminServer(cmd)
	case "show_api_server":
		return c.ShowAPIServer(cmd)
	case "list_api_server":
		return c.ListAPIServer(cmd)
	case "add_api_server":
		return c.AddAPIServer(cmd)
	case "delete_api_server":
		return c.DeleteAPIServer(cmd)
	case "add_admin_server":
		return nil, fmt.Errorf("cannot add admin server in admin mode")
	case "delete_admin_server":
		return nil, fmt.Errorf("cannot delete admin server in admin mode")
	case "save_config_command":
		return c.SaveServerConfig(cmd)
	case "use_api_server":
		return c.UseAPIServer(cmd)
	case "use_admin_server":
		return c.UseAdminServer(cmd)
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}
}
func (c *CLI) ExecuteUserCommand(cmd *Command) (ResponseIf, error) {
	switch cmd.Type {
	case "register_user":
		return c.RegisterUser(cmd)
	case "login_user":
		return c.LoginUserByCommand(cmd)
	case "logout":
		return c.Logout()
	case "ping":
		return c.PingByCommand(cmd)
	// Configuration commands
	case "list_configs":
		return c.ListConfigs(cmd)
	case "set_log_level":
		return c.SetLogLevel(cmd)
	case "benchmark":
		return c.RunBenchmark(cmd)
	case "list_datasets":
		return c.ListDatasets(cmd)
	case "list_dataset_documents":
		return c.ListDatasetDocumentUserCommand(cmd)
	case "search_on_datasets":
		return c.SearchOnDatasets(cmd)
	case "search_help":
		printSearchHelp()
		return nil, nil
	case "create_token":
		return c.CreateToken(cmd)
	case "list_tokens":
		return c.ListTokens(cmd)
	case "drop_token":
		return c.DropToken(cmd)
	case "set_token":
		return c.SetToken(cmd)
	case "show_token":
		return c.ShowToken(cmd)
	case "unset_token":
		return c.UnsetToken(cmd)
	case "show_version":
		return c.ShowServerVersion(cmd)
	case "show_current":
		return c.ShowCommonCurrent(cmd)
	case "list_available_providers":
		return c.ListAvailableProviders(cmd)
	case "show_provider":
		return c.ShowProvider(cmd)
	case "list_provider_models":
		return c.ListModels(cmd)
	case "list_supported_models":
		return c.ListSupportedModels(cmd)
	case "list_instance_models":
		return c.ListInstanceModels(cmd)
	case "show_provider_model":
		return c.ShowProviderModel(cmd)
	case "show_model":
		return c.ShowModel(cmd)
	case "list_all_models":
		return c.ListAllModels(cmd)
	// Provider commands
	case "add_provider":
		return c.AddProvider(cmd)
	case "list_providers":
		return c.ListProviders(cmd)
	case "delete_provider":
		return c.DeleteProvider(cmd)
	// Provider instance commands
	case "create_provider_instance":
		return c.CreateProviderInstance(cmd)
	case "list_provider_instances":
		return c.ListProviderInstances(cmd)
	case "show_provider_instance":
		return c.ShowProviderInstance(cmd)
	case "show_instance_balance":
		return c.ShowInstanceBalance(cmd)
	case "alter_provider_instance":
		return c.AlterProviderInstance(cmd)
	case "drop_provider_instance":
		return c.DropProviderInstance(cmd)
	case "drop_instance_model":
		return c.DropInstanceModel(cmd)
	case "enable_model":
		return c.EnableOrDisableModel(cmd, "enable")
	case "disable_model":
		return c.EnableOrDisableModel(cmd, "disable")
	case "add_custom_model":
		return c.AddCustomModel(cmd)
	case "chat_to_model":
		return c.ChatToModel(cmd)
	case "think_chat_to_model":
		return c.ChatToModel(cmd)
	case "embed_user_text":
		return c.EmbedUserText(cmd)
	case "rarank_user_document":
		return c.RerankUserDocument(cmd)
	case "tts_user_command":
		return c.TTSUserCommand(cmd)
	case "asr_user_command":
		return c.ASRUserCommand(cmd)
	case "ocr_user_command":
		return c.OCRUserCommand(cmd)
	case "parse_file_user_command":
		return c.ParseFileUserCommand(cmd)
	case "check_provider_connection":
		return c.CheckProviderConnection(cmd)
	case "check_provider_with_key":
		return c.CheckProviderWithKey(cmd)
	case "use_model":
		return c.UseModel(cmd)
	case "use_api_server":
		return c.UseAPIServer(cmd)
	case "use_admin_server":
		return c.UseAdminServer(cmd)
	case "set_default_model":
		return c.SetDefaultModel(cmd)
	case "reset_default_model":
		return c.ResetDefaultModel(cmd)
	case "list_user_default_models":
		return c.ListDefaultModels(cmd)
	case "list_tasks_user_command":
		return c.ListTasksUserCommand(cmd)
	case "show_task_user_command":
		return c.ShowTaskUserCommand(cmd)
	case "create_chunk_store":
		return c.CreateChunkStore(cmd)
	case "drop_chunk_store":
		return c.DropChunkStore(cmd)
	case "create_metadata_store":
		return c.CreateMetadataStore(cmd)
	case "drop_metadata_store":
		return c.DropMetadataStore(cmd)
	case "insert_chunks_from_file":
		return c.InsertChunksFromFile(cmd)
	case "insert_metadata_from_file":
		return c.InsertMetadataFromFile(cmd)
	case "update_chunk":
		return c.UpdateChunk(cmd)
	case "get_chunk":
		return c.GetChunk(cmd)
	case "set_meta":
		return c.SetMeta(cmd)
	case "delete_meta":
		return c.DeleteMeta(cmd)
	case "rm_tags":
		return c.RmTags(cmd)
	case "remove_chunks":
		return c.RemoveChunks(cmd)
	case "get_metadata":
		return c.GetMetadata(cmd)
	case "parse_documents_user_command":
		return c.ParseDocumentsUserCommand(cmd)
	case "user_parse_local_file_command":
		return c.UserParseLocalFile(cmd)
	case "show_admin_server":
		return c.ShowAdminServer(cmd)
	case "show_api_server":
		return c.ShowAPIServer(cmd)
	case "list_api_server":
		return c.ListAPIServer(cmd)
	case "add_api_server":
		return c.AddAPIServer(cmd)
	case "delete_api_server":
		return c.DeleteAPIServer(cmd)
	case "add_admin_server":
		return c.AddAdminServer(cmd)
	case "delete_admin_server":
		return c.DeleteAdminServer(cmd)
	case "user_chunk_command":
		return c.ChunkCommand(cmd)
	case "save_config_command":
		return c.SaveServerConfig(cmd)
	case "file_system_command":
		return c.executeFilesystem(cmd)
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}

}
