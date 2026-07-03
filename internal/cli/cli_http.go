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
	case "admin_login_user":
		return c.LoginUserByCommand(cmd)
	case "admin_logout":
		return c.Logout()
	case "admin_ping_server":
		return c.PingByCommand(cmd)
	case "benchmark":
		return c.RunBenchmark(cmd)
	case "admin_list_services":
		return c.AdminListServicesCommand(cmd)
	case "admin_start_service":
		return c.AdminStartServiceCommand(cmd)
	case "admin_restart_service":
		return c.AdminRestartServiceCommand(cmd)
	case "admin_shutdown_service":
		return c.AdminShutdownServiceCommand(cmd)
	case "admin_grant_user_admin":
		return c.AdminGrantUserAdminCommand(cmd)
	case "admin_revoke_user_admin":
		return c.AdminRevokeUserAdminCommand(cmd)
	case "admin_grant_role_permission":
		return c.AdminGrantRolePermissionCommand(cmd)
	case "admin_revoke_role_permission":
		return c.AdminRevokeRolePermissionCommand(cmd)
	case "admin_show_role_permission":
		return c.AdminShowRolePermissionCommand(cmd)
	case "admin_create_user":
		return c.AdminCreateUserCommand(cmd)
	case "admin_create_user_api_key":
		return c.AdminCreateUserAPIKeyCommand(cmd)
	case "admin_create_role":
		return c.AdminCreateRoleCommand(cmd)
	case "admin_activate_user":
		return c.AdminActivateUser(cmd)
	case "admin_alter_user":
		return c.AdminAlterUserPassword(cmd)
	case "admin_alter_role":
		return c.AdminAlterRole(cmd)
	case "admin_alter_provider_instance":
		return c.CommonAlterProviderInstanceCommand(cmd)
	case "admin_drop_user":
		return c.AdminDropUserCommand(cmd)
	case "admin_drop_user_api_key":
		return c.AdminDropUserAPIKeyCommand(cmd)
	case "admin_drop_role":
		return c.AdminDropRoleCommand(cmd)
	case "admin_show_service":
		return c.AdminShowService(cmd)
	case "admin_show_version_command":
		return c.AdminShowVersionCommand(cmd)
	case "admin_show_current":
		return c.CommonShowCurrentCommand(cmd)
	case "admin_list_variables":
		return c.AdminListVariablesCommand(cmd)
	case "admin_list_configs":
		return c.AdminListConfigsCommand(cmd)
	case "admin_list_environments":
		return c.AdminListEnvironmentsCommand(cmd)
	case "admin_show_variable":
		return c.AdminShowVariable(cmd)
	case "admin_set_license":
		return c.AdminSetLicenseCommand(cmd)
	case "admin_set_license_config":
		return c.AdminSetLicenseConfigCommand(cmd)
	case "admin_set_variable":
		return c.AdminSetVariableCommand(cmd)
	case "admin_set_role_default_model":
		return c.AdminSetRoleDefaultModelsCommand(cmd)
	case "admin_set_log_level":
		return c.AdminSetLogLevelCommand(cmd)
	case "admin_reset_role_default_model":
		return c.AdminResetRoleDefaultModelsCommand(cmd)
	case "list_user_datasets":
		return c.ListUserDatasets(cmd)
	case "admin_list_resources_command":
		return c.AdminListResourcesCommand(cmd)
	case "admin_list_roles_command":
		return c.AdminListRolesCommand(cmd)
	case "admin_list_available_providers":
		return c.CommonAvailableProvidersCommand(cmd)
	case "admin_show_provider":
		return c.CommonShowProviderCommand(cmd)
	case "admin_show_provider_instance":
		return c.CommonShowProviderInstanceCommand(cmd)
	case "admin_show_provider_instance_balance":
		return c.CommonShowProviderInstanceBalanceCommand(cmd)
	case "admin_show_provider_model":
		return c.CommonShowProviderModelCommand(cmd)
	case "admin_list_provider_models":
		return c.CommonListModelsCommand(cmd)
	case "admin_list_provider_instance_models":
		return c.CommonListInstanceModelsCommand(cmd)
	case "admin_list_provider_instances":
		return c.CommonListProviderInstancesCommand(cmd)
	case "admin_show_model":
		return c.CommonShowModelCommand(cmd)
	case "admin_list_providers":
		return c.AdminListProvidersCommand(cmd)
	case "admin_list_all_models":
		return c.CommonListAllModels(cmd)
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
	case "admin_check_license":
		return c.AdminCheckLicenseCommand(cmd)
	case "admin_check_provider_with_key":
		return c.CommonCheckProviderWithKeyCommand(cmd)
	case "admin_check_provider_instance":
		return c.CommonCheckProviderConnectionCommand(cmd)
	case "admin_show_fingerprint":
		return c.AdminShowFingerprintCommand(cmd)
	case "admin_show_license":
		return c.AdminShowLicenseCommand(cmd)
	case "admin_show_user":
		return c.AdminShowUserCommand(cmd)
	case "admin_show_role":
		return c.AdminShowRoleCommand(cmd)
	case "admin_show_role_default_models":
		return c.AdminShowRoleDefaultModelsCommand(cmd)
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
	case "admin_show_users_plan_command":
		return c.AdminShowUsersPlanCommand(cmd)
	case "admin_list_users_command":
		return c.AdminListUsersCommand(cmd)
	case "admin_list_users_condition_command":
		return c.AdminListUsersConditionCommand(cmd)
	case "admin_show_quota_summary":
		return c.AdminShowQuotaSummaryCommand(cmd)
	case "admin_show_tasks_summary":
		return c.AdminShowTasksSummaryCommand(cmd)
	case "admin_show_data_summary":
		return c.AdminShowDataSummaryCommand(cmd)
	case "admin_show_data_orphan":
		return c.AdminShowDataOrphanCommand(cmd)
	case "admin_show_data_storage":
		return c.AdminShowDataStorageCommand(cmd)
	case "admin_show_data_index":
		return c.AdminShowDataIndexCommand(cmd)
	case "admin_purge_orphan_command":
		return c.AdminPurgeOrphanCommand(cmd)
	case "admin_purge_user_command":
		return c.AdminPurgeUserCommand(cmd)
	case "admin_purge_users_command":
		return c.AdminPurgeUsersCommand(cmd)
	case "admin_list_user_ingestion_tasks":
		return c.AdminListUserIngestionTasksCommand(cmd)
	case "admin_list_user_datasets":
		return c.AdminListUserDatasetsCommand(cmd)
	case "admin_list_user_agents":
		return c.AdminListUserAgentsCommand(cmd)
	case "admin_list_user_chats":
		return c.AdminListUserChatsCommand(cmd)
	case "admin_list_user_searches":
		return c.AdminListUserSearchesCommand(cmd)
	case "admin_list_user_models":
		return c.AdminListUserModelsCommand(cmd)
	case "admin_list_user_files":
		return c.AdminListUserFilesCommand(cmd)
	case "admin_list_user_keys":
		return c.AdminListUserKeysCommand(cmd)
	case "admin_list_user_providers":
		return c.AdminListUserProvidersCommand(cmd)
	case "admin_list_user_provider_instances":
		return c.AdminListUserProviderInstancesCommand(cmd)
	case "admin_list_user_provider_instance_models":
		return c.AdminListUserProviderInstanceModelsCommand(cmd)
	case "admin_list_user_default_models":
		return c.AdminListUserDefaultModelsCommand(cmd)
	case "admin_stop_user_ingestion_tasks_command":
		return c.AdminStopUserIngestionTasksCommand(cmd)
	case "admin_remove_user_ingestion_tasks_command":
		return c.AdminRemoveUserIngestionTasksCommand(cmd)
	case "admin_add_provider":
		return c.AdminAddProviderCommand(cmd)
	case "admin_add_model_instance":
		return c.AdminAddModelInstanceCommand(cmd)
	case "admin_add_models":
		return c.AdminAddModelsCommand(cmd)
	case "admin_delete_model_providers":
		return c.AdminDeleteProvidersCommand(cmd)
	case "admin_delete_model_instance":
		return c.AdminDeleteInstancesCommand(cmd)
	case "admin_delete_model":
		return c.AdminDeleteModelsCommand(cmd)
	case "admin_enable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "enable")
	case "admin_disable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "disable")
	case "admin_show_admin_server":
		return c.CommonShowAdminServerCommand(cmd)
	case "admin_show_api_server":
		return c.CommonShowAPIServerCommand(cmd)
	case "admin_show_log_level":
		return c.AdminShowLogLevelCommand(cmd)
	case "admin_list_api_servers":
		return c.CommonListAPIServersCommand(cmd)
	case "api_add_api_server":
		return c.AddAPIServerCommand(cmd)
	case "api_delete_api_server":
		return c.DeleteAPIServerCommand(cmd)
	case "api_add_admin_server":
		return nil, fmt.Errorf("cannot add admin server in admin mode")
	case "api_delete_admin_server":
		return nil, fmt.Errorf("cannot delete admin server in admin mode")
	case "admin_save_config_command":
		return c.CommonSaveServerConfigCommand(cmd)
	case "admin_use_api_server":
		return c.CommonUseAPIServerCommand(cmd)
	case "admin_use_admin_server":
		return c.CommonUseAdminServerCommand(cmd)
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}
}
func (c *CLI) ExecuteUserCommand(cmd *Command) (ResponseIf, error) {
	switch cmd.Type {
	case "api_register_user":
		return c.RegisterUser(cmd)
	case "api_login_user":
		return c.LoginUserByCommand(cmd)
	case "api_logout":
		return c.Logout()
	case "api_ping_server":
		return c.PingByCommand(cmd)
	case "api_list_configs":
		return c.ListConfigs(cmd)
	case "api_set_log_level":
		return c.APISetLogLevelCommand(cmd)
	case "benchmark":
		return c.RunBenchmark(cmd)
	case "api_list_datasets":
		return c.APIListDatasetsCommand(cmd)
	case "api_list_dataset_documents":
		return c.APIListDatasetDocumentsCommand(cmd)
	case "api_list_dataset_files":
		return c.APIListDatasetFilesCommand(cmd)
	case "api_list_agents":
		return c.APIListAgentsCommand(cmd)
	case "api_list_chats":
		return c.APIListChatsCommand(cmd)
	case "api_list_searches":
		return c.APIListSearchesCommand(cmd)
	case "api_list_memories":
		return c.APIListMemoriesCommand(cmd)
	case "search_on_datasets":
		return c.SearchOnDatasets(cmd)
	case "search_help":
		printSearchHelp()
		return nil, nil
	case "api_create_api_key":
		return c.APICreateAPIKeyCommand(cmd)
	case "api_create_dataset":
		return c.APICreateDatasetCommand(cmd)
	case "api_create_agent":
		return c.APICreateAgentCommand(cmd)
	case "api_create_chat":
		return c.APICreateChatCommand(cmd)
	case "api_create_search":
		return c.APICreateSearchCommand(cmd)
	case "api_create_memory":
		return c.APICreateMemoryCommand(cmd)
	case "api_list_api_keys":
		return c.APIListAPIKeysCommand(cmd)
	case "api_delete_api_key":
		return c.APIDeleteAPIKeyCommand(cmd)
	case "api_set_api_key":
		return c.APISetAPIKeyCommand(cmd)
	case "api_set_variable":
		return c.APISetVariableCommand(cmd)
	case "api_show_variable":
		return c.APIShowVariableCommand(cmd)
	case "api_unset_api_key":
		return c.APIUnsetAPIKeyCommand(cmd)
	case "api_show_version":
		return c.APIShowVersionCommand(cmd)
	case "api_show_api_key":
		return c.APIShowAPIKeyCommand(cmd)
	case "api_show_current":
		return c.CommonShowCurrentCommand(cmd)
	case "api_list_available_providers":
		return c.CommonAvailableProvidersCommand(cmd)
	case "api_show_provider":
		return c.CommonShowProviderCommand(cmd)
	case "api_show_provider_instance":
		return c.CommonShowProviderInstanceCommand(cmd)
	case "api_show_provider_instance_balance":
		return c.CommonShowProviderInstanceBalanceCommand(cmd)
	case "api_show_provider_instance_task":
		return c.APIShowProviderInstanceTaskCommand(cmd)
	case "api_show_provider_model":
		return c.CommonShowProviderModelCommand(cmd)
	case "api_list_provider_models":
		return c.CommonListModelsCommand(cmd)
	case "api_list_provider_instance_models":
		return c.CommonListInstanceModelsCommand(cmd)
	case "api_list_provider_instance_models_sync":
		return c.CommonListInstanceModelsSyncCommand(cmd)
	case "api_list_provider_instance_tasks":
		return c.APIListModelInstanceTasksCommand(cmd)

	// Provider commands
	case "api_show_model":
		return c.CommonShowModelCommand(cmd)
	case "api_list_all_models":
		return c.CommonListAllModels(cmd)
	case "api_add_provider":
		return c.APIAddProviderCommand(cmd)
	case "api_list_providers":
		return c.APIListProvidersCommand(cmd)
	case "api_delete_provider":
		return c.APIDeleteProviderCommand(cmd)
	case "api_delete_provider_instance":
		return c.APIDeleteProviderInstanceCommand(cmd)
	case "api_drop_dataset":
		return c.APIDropDatasetCommand(cmd)
	case "api_drop_chat":
		return c.APIDropChatCommand(cmd)
	case "api_drop_search":
		return c.APIDropSearchCommand(cmd)
	case "api_drop_memory":
		return c.APIDropMemoryCommand(cmd)
	case "api_drop_agent":
		return c.APIDropAgentCommand(cmd)
	case "api_add_provider_instance":
		return c.APIAddProviderInstanceCommand(cmd)
	case "api_list_provider_instances":
		return c.CommonListProviderInstancesCommand(cmd)
	case "api_alter_provider_instance":
		return c.CommonAlterProviderInstanceCommand(cmd)
	case "api_delete_provider_instance_model":
		return c.APIDeleteProviderInstanceModelCommand(cmd)
	case "enable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "enable")
	case "disable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "disable")
	case "api_add_custom_model":
		return c.APIAddCustomModelCommand(cmd)
	case "api_chat_to_model":
		return c.APIChatToModelCommand(cmd)
	case "api_openai_chat":
		return c.APIOpenaiChatCommand(cmd)
	case "openai_chat_help":
		printOpenaiChatHelp()
		return nil, nil
	case "api_embed_user_text":
		return c.EmbedUserTextCommand(cmd)
	case "api_rarank_user_document":
		return c.APIRerankUserDocumentCommand(cmd)
	case "chat completions":
		return c.ChatCompletions(cmd)
	case "chat completions help":
		printChatCompletionsHelp()
		return nil, nil
	case "tts_user_command":
		return c.APITTSUserCommand(cmd)
	case "asr_user_command":
		return c.APIASRUserCommand(cmd)
	case "ocr_user_command":
		return c.APIOCRUserCommand(cmd)
	case "api_model_parse_file":
		return c.APIModelParseFileCommand(cmd)
	case "check_provider_connection":
		return c.CommonCheckProviderConnectionCommand(cmd)
	case "check_provider_with_key":
		return c.CommonCheckProviderWithKeyCommand(cmd)
	case "api_use_model":
		return c.APIUseModelCommand(cmd)
	case "api_use_api_server":
		return c.CommonUseAPIServerCommand(cmd)
	case "api_use_admin_server":
		return c.CommonUseAdminServerCommand(cmd)
	case "api_set_default_model":
		return c.APISetDefaultModelCommand(cmd)
	case "api_reset_default_model":
		return c.APIResetDefaultModelCommand(cmd)
	case "api_list_default_models":
		return c.APIListDefaultModelsCommand(cmd)
	case "api_parse_documents":
		return c.APIParseDocumentsCommand(cmd)
	case "api_start_ingestion":
		return c.APIStartIngestionCommand(cmd)
	case "api_stop_ingestion":
		return c.APIStopIngestionCommand(cmd)
	case "api_list_ingestion_tasks":
		return c.APIListIngestionTasks(cmd)
	case "api_remove_task":
		return c.APIRemoveTaskCommand(cmd)
	case "user_parse_local_file_command":
		return c.APIParseLocalFileCommand(cmd)
	case "api_show_admin_server":
		return c.CommonShowAdminServerCommand(cmd)
	case "api_show_api_server":
		return c.CommonShowAPIServerCommand(cmd)
	case "api_show_log_level":
		return c.APIShowLogLevelCommand(cmd)
	case "api_list_api_servers":
		return c.CommonListAPIServersCommand(cmd)
	case "api_list_environments":
		return c.APIListEnvironmentsCommand(cmd)
	case "api_list_variables":
		return c.APIListVariablesCommand(cmd)
	case "api_add_api_server":
		return c.AddAPIServerCommand(cmd)
	case "api_delete_api_server":
		return c.DeleteAPIServerCommand(cmd)
	case "api_add_admin_server":
		return c.AddAdminServerCommand(cmd)
	case "api_delete_admin_server":
		return c.DeleteAdminServerCommand(cmd)
	case "api_save_config_command":
		return c.CommonSaveServerConfigCommand(cmd)

	// File system commands
	case "file_system_command":
		return c.ExecuteFilesystemCommand(cmd)

	// For debug
	case "dev_chunk":
		return c.DevChunkCommand(cmd)
	case "dev_create_chunk_store":
		return c.DevCreateChunkStoreCommand(cmd)
	case "dev_drop_chunk_store":
		return c.DevDropChunkStoreCommand(cmd)
	case "dev_create_metadata_store":
		return c.DevCreateMetadataStoreCommand(cmd)
	case "dev_drop_metadata_store":
		return c.DevDropMetadataStoreCommand(cmd)
	case "dev_insert_chunks_from_file":
		return c.DevInsertChunksFromFileCommand(cmd)
	case "dev_insert_metadata_from_file":
		return c.DevInsertMetadataFromFileCommand(cmd)
	case "dev_update_chunk":
		return c.DevUpdateChunkCommand(cmd)
	case "dev_get_chunk":
		return c.DevGetChunkCommand(cmd)
	case "dev_set_meta":
		return c.DevSetMetaCommand(cmd)
	case "dev_delete_meta":
		return c.DevDeleteMetaCommand(cmd)
	case "dev_rm_tags":
		return c.DevRmTagsCommand(cmd)
	case "dev_remove_chunks":
		return c.DevRemoveChunksCommand(cmd)
	case "dev_get_metadata":
		return c.DevGetMetadataCommand(cmd)
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}

}
