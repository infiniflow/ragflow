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
)

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
	case "ping_server":
		return c.PingByCommand(cmd)
	case "benchmark":
		return c.RunBenchmark(cmd)
	case "admin_list_services":
		return c.AdminListServicesCommand(cmd)
	case "grant_admin":
		return c.GrantAdmin(cmd)
	case "revoke_admin":
		return c.RevokeAdmin(cmd)
	case "admin_create_user_command":
		return c.AdminCreateUserCommand(cmd)
	case "admin_create_user_api_key_command":
		return c.AdminCreateUserAPIKeyCommand(cmd)
	case "admin_create_role_command":
		return c.AdminCreateRoleCommand(cmd)
	case "activate_user":
		return c.ActivateUser(cmd)
	case "alter_user":
		return c.AlterUserPassword(cmd)
	case "admin_drop_user_command":
		return c.AdminDropUserCommand(cmd)
	case "admin_drop_user_api_key_command":
		return c.AdminDropUserAPIKeyCommand(cmd)
	case "admin_show_service":
		return c.AdminShowService(cmd)
	case "admin_show_version_command":
		return c.AdminShowVersionCommand(cmd)
	case "admin_show_current":
		return c.CommonShowCurrent(cmd)
	case "admin_list_variables":
		return c.AdminListVariables(cmd)
	case "admin_show_variable":
		return c.AdminShowVariable(cmd)
	case "admin_set_license_command":
		return c.AdminSetLicenseCommand(cmd)
	case "admin_set_license_config_command":
		return c.AdminSetLicenseConfigCommand(cmd)
	case "set_variable":
		return c.SetVariable(cmd)
	case "list_user_datasets":
		return c.ListUserDatasets(cmd)
	case "admin_list_resources_command":
		return c.AdminListResourcesCommand(cmd)
	case "admin_list_roles_command":
		return c.AdminListRolesCommand(cmd)
	case "generate_token":
		return c.GenerateAdminToken(cmd)
	case "list_tokens":
		return c.ListAdminTokens(cmd)
	case "list_available_providers":
		return c.ListAvailableProviders(cmd)
	case "admin_show_provider":
		return c.CommonShowProviderCommand(cmd)
	case "admin_show_provider_model":
		return c.CommonShowProviderModelCommand(cmd)
	case "admin_list_provider_models":
		return c.CommonListModelsCommand(cmd)
	case "list_supported_models":
		return c.ListSupportedModels(cmd)
	case "list_instance_models":
		return c.ListInstanceModels(cmd)
	case "admin_show_model":
		return c.CommonShowModel(cmd)
	case "admin_list_all_models":
		return c.ListAllModels(cmd)
	case "list_admin_tasks":
		return c.ListAdminTasks(cmd)
	case "admin_list_ingestors":
		return c.ListAdminIngestors(cmd)
	case "admin_stop_ingestion_tasks":
		return c.AdminStopIngestionCommand(cmd)
	case "admin_remove_ingestion_tasks":
		return c.AdminRemoveIngestionCommand(cmd)
	case "admin_shutdown_ingestor_command":
		return c.AdminShutdownIngestor(cmd)
	case "list_admin_ingestion_tasks":
		return c.ListAdminIngestionTasks(cmd)
	case "user_list_message_queue_command":
		return c.UserListMessageQueueCommand(cmd)
	case "user_publish_message_command":
		return c.UserPublishMessageCommand(cmd)
	case "user_pull_message_command":
		return c.UserPullMessageCommand(cmd)
	case "user_show_message_queue_command":
		return c.UserShowMessageQueueCommand(cmd)
	case "admin_remove_service_command":
		return c.AdminRemoveServiceCommand(cmd)
	case "admin_check_license_command":
		return c.AdminCheckLicenseCommand(cmd)
	case "admin_show_fingerprint":
		return c.AdminShowFingerprintCommand(cmd)
	case "admin_show_license":
		return c.AdminShowLicenseCommand(cmd)
	case "admin_show_user":
		return c.AdminShowUserCommand(cmd)
	case "admin_show_role":
		return c.AdminShowRoleCommand(cmd)
	case "admin_show_user_activity_command":
		return c.AdminShowUserActivityCommand(cmd)
	case "admin_show_user_summary_command":
		return c.AdminShowUserSummaryCommand(cmd)
	case "admin_show_user_dataset_command":
		return c.AdminShowUserDatasetCommand(cmd)
	case "admin_show_user_storage_command":
		return c.AdminShowUserStorageCommand(cmd)
	case "admin_show_user_quota_command":
		return c.AdminShowUserQuotaCommand(cmd)
	case "admin_show_user_index_command":
		return c.AdminShowUserIndexCommand(cmd)
	case "admin_show_user_permission_command":
		return c.AdminShowUserPermissionCommand(cmd)
	case "admin_show_users_summary_command":
		return c.AdminShowUsersSummaryCommand(cmd)
	case "admin_show_users_activity_command":
		return c.AdminShowUsersActivityCommand(cmd)
	case "admin_list_users_command":
		return c.AdminListUsersCommand(cmd)
	case "admin_list_users_condition_command":
		return c.AdminListUsersConditionCommand(cmd)
	case "admin_show_quota_summary_command":
		return c.AdminShowQuotaSummaryCommand(cmd)
	case "admin_show_tasks_summary_command":
		return c.AdminShowTasksSummaryCommand(cmd)
	case "admin_show_data_summary_command":
		return c.AdminShowDataSummaryCommand(cmd)
	case "admin_show_data_orphan_command":
		return c.AdminShowDataOrphanCommand(cmd)
	case "admin_show_data_storage_command":
		return c.AdminShowDataStorageCommand(cmd)
	case "admin_show_data_index_command":
		return c.AdminShowDataIndexCommand(cmd)
	case "admin_purge_orphan_command":
		return c.AdminPurgeOrphanCommand(cmd)
	case "admin_purge_user_command":
		return c.AdminPurgeUserCommand(cmd)
	case "admin_purge_users_command":
		return c.AdminPurgeUsersCommand(cmd)
	case "admin_list_user_ingestion_tasks_command":
		return c.AdminListUserIngestionTasksCommand(cmd)
	case "admin_list_user_datasets_command":
		return c.AdminListUserDatasetsCommand(cmd)
	case "admin_list_user_agents_command":
		return c.AdminListUserAgentsCommand(cmd)
	case "admin_list_user_chats_command":
		return c.AdminListUserChatsCommand(cmd)
	case "admin_list_user_searches_command":
		return c.AdminListUserSearchesCommand(cmd)
	case "admin_list_user_models_command":
		return c.AdminListUserModelsCommand(cmd)
	case "admin_list_user_files_command":
		return c.AdminListUserFilesCommand(cmd)
	case "admin_list_user_keys_command":
		return c.AdminListUserKeysCommand(cmd)
	case "admin_stop_user_ingestion_tasks_command":
		return c.AdminStopUserIngestionTasksCommand(cmd)
	case "admin_remove_user_ingestion_tasks_command":
		return c.AdminRemoveUserIngestionTasksCommand(cmd)
	// TODO: Implement other commands
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
	case "ping_server":
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
		return c.CommonShowCurrent(cmd)
	case "list_available_providers":
		return c.ListAvailableProviders(cmd)
	case "show_provider":
		return c.CommonShowProviderCommand(cmd)
	case "list_provider_models":
		return c.CommonListModelsCommand(cmd)
	case "list_supported_models":
		return c.ListSupportedModels(cmd)
	case "list_instance_models":
		return c.ListInstanceModels(cmd)
	case "show_provider_model":
		return c.CommonShowProviderModelCommand(cmd)
	case "show_model":
		return c.CommonShowModel(cmd)
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
	case "openai_chat":
		return c.OpenaiChat(cmd)
	case "openai_chat_help":
		printOpenaiChatHelp()
		return nil, nil
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
	case "user_start_ingestion_command":
		return c.UserStartIngestionCommand(cmd)
	case "user_stop_ingestion_command":
		return c.UserStopIngestionCommand(cmd)
	case "user_list_ingestion_tasks":
		return c.ListUserIngestionTasks(cmd)
	case "user_remove_task_command":
		return c.UserRemoveTaskCommand(cmd)
	// TODO: Implement other commands
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
		return c.ExecuteFilesystemCommand(cmd)
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}

}
