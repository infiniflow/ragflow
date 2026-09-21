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
func (c *CLI) ExecuteCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	switch c.Config.CLIMode {
	case APIMode:
		// Interactive mode: execute command with user privileges
		return c.ExecuteUserCommand(commandCount, cmd)
	case AdminMode:
		// Admin mode: execute command with admin privileges
		return c.ExecuteAdminCommand(commandCount, cmd)
	default:
		return nil, fmt.Errorf("invalid server type: %s", c.Config.CLIMode)
	}
}

func (c *CLI) ExecuteAdminCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	switch cmd.Type {
	case "admin_login_user":
		return c.LoginUserByCommand(commandCount, cmd)
	case "admin_logout":
		return c.Logout()
	case "admin_ping_store":
		return c.AdminPingStoreCommand(commandCount, cmd)
	case "admin_ping_engine":
		return c.AdminPingEngineCommand(commandCount, cmd)
	case "admin_ping_mq":
		return c.AdminPingMQCommand(commandCount, cmd)
	case "admin_ping_cache":
		return c.AdminPingCacheCommand(commandCount, cmd)
	case "admin_ping_server":
		return c.PingServerByCommand(commandCount, cmd)
	case "admin_live_server":
		return c.AdminLiveServerCommand(commandCount, cmd)
	case "admin_health_server":
		return c.AdminHealthServerCommand(commandCount, cmd)
	case "benchmark":
		return c.RunBenchmark(commandCount, cmd)
	case "admin_list_services":
		return c.AdminListServicesCommand(commandCount, cmd)
	case "admin_start_service":
		return c.AdminStartServiceCommand(commandCount, cmd)
	case "admin_restart_service":
		return c.AdminRestartServiceCommand(commandCount, cmd)
	case "admin_shutdown_service":
		return c.AdminShutdownServiceCommand(commandCount, cmd)
	case "admin_grant_user_admin":
		return c.AdminGrantUserAdminCommand(commandCount, cmd)
	case "admin_revoke_user_admin":
		return c.AdminRevokeUserAdminCommand(commandCount, cmd)
	case "admin_grant_role_permission":
		return c.AdminGrantRolePermissionCommand(commandCount, cmd)
	case "admin_revoke_role_permission":
		return c.AdminRevokeRolePermissionCommand(commandCount, cmd)
	case "admin_show_role_permission":
		return c.AdminShowRolePermissionCommand(commandCount, cmd)
	case "admin_create_user":
		return c.AdminCreateUserCommand(commandCount, cmd)
	case "admin_create_user_api_key":
		return c.AdminCreateUserAPIKeyCommand(commandCount, cmd)
	case "admin_create_role":
		return c.AdminCreateRoleCommand(commandCount, cmd)
	case "admin_activate_user":
		return c.AdminActivateUser(commandCount, cmd)
	case "admin_alter_user":
		return c.AdminAlterUserPassword(commandCount, cmd)
	case "admin_alter_role":
		return c.AdminAlterRole(commandCount, cmd)
	case "admin_alter_provider_instance":
		return c.CommonAlterProviderInstanceCommand(commandCount, cmd)
	case "admin_drop_user":
		return c.AdminDropUserCommand(commandCount, cmd)
	case "admin_drop_user_api_key":
		return c.AdminDropUserAPIKeyCommand(commandCount, cmd)
	case "admin_drop_role":
		return c.AdminDropRoleCommand(commandCount, cmd)
	case "admin_show_service":
		return c.AdminShowService(commandCount, cmd)
	case "admin_show_version_command":
		return c.AdminShowVersionCommand(commandCount, cmd)
	case "admin_show_current":
		return c.CommonShowCurrentCommand(commandCount, cmd)
	case "admin_list_variables":
		return c.AdminListVariablesCommand(commandCount, cmd)
	case "admin_list_configs":
		return c.AdminListConfigsCommand(commandCount, cmd)
	case "admin_list_environments":
		return c.AdminListEnvironmentsCommand(commandCount, cmd)
	case "admin_show_variable":
		return c.AdminShowVariable(commandCount, cmd)
	case "admin_set_license":
		return c.AdminSetLicenseCommand(commandCount, cmd)
	case "admin_set_soft_fingerprint":
		return c.AdminSetSoftFingerprintCommand(commandCount, cmd)
	case "admin_set_license_config":
		return c.AdminSetLicenseConfigCommand(commandCount, cmd)
	case "admin_set_variable":
		return c.AdminSetVariableCommand(commandCount, cmd)
	case "admin_set_role_default_model":
		return c.AdminSetRoleDefaultModelsCommand(commandCount, cmd)
	case "admin_set_log_level":
		return c.AdminSetLogLevelCommand(commandCount, cmd)
	case "admin_reset_role_default_model":
		return c.AdminResetRoleDefaultModelsCommand(commandCount, cmd)
	case "list_user_datasets":
		return c.ListUserDatasets(commandCount, cmd)
	case "admin_list_resources_command":
		return c.AdminListResourcesCommand(commandCount, cmd)
	case "admin_list_roles_command":
		return c.AdminListRolesCommand(commandCount, cmd)
	case "admin_list_available_providers":
		return c.CommonAvailableProvidersCommand(commandCount, cmd)
	case "admin_show_provider":
		return c.CommonShowProviderCommand(commandCount, cmd)
	case "admin_show_provider_instance":
		return c.CommonShowProviderInstanceCommand(commandCount, cmd)
	case "admin_show_provider_instance_balance":
		return c.CommonShowProviderInstanceBalanceCommand(commandCount, cmd)
	case "admin_show_provider_model":
		return c.CommonShowProviderModelCommand(commandCount, cmd)
	case "admin_list_provider_models":
		return c.CommonListModelsCommand(commandCount, cmd)
	case "admin_list_provider_instance_models":
		return c.CommonListInstanceModelsCommand(commandCount, cmd)
	case "admin_list_provider_instances":
		return c.CommonListProviderInstancesCommand(commandCount, cmd)
	case "admin_show_model":
		return c.CommonShowModelCommand(commandCount, cmd)
	case "admin_list_providers":
		return c.AdminListProvidersCommand(commandCount, cmd)
	case "admin_list_all_models":
		return c.CommonListAllModels(commandCount, cmd)
	case "list_admin_tasks":
		return c.ListAdminTasks(commandCount, cmd)
	case "admin_list_ingestors":
		return c.ListAdminIngestors(commandCount, cmd)
	case "admin_stop_ingestion_tasks":
		return c.AdminStopIngestionCommand(commandCount, cmd)
	case "admin_remove_ingestion_tasks":
		return c.AdminRemoveIngestionCommand(commandCount, cmd)
	case "admin_shutdown_ingestor_command":
		return c.AdminShutdownIngestor(commandCount, cmd)
	case "list_admin_ingestion_tasks":
		return c.ListAdminIngestionTasks(commandCount, cmd)
	case "user_list_message_queue_command":
		return c.UserListMessageQueueCommand(commandCount, cmd)
	case "user_publish_message_command":
		return c.UserPublishMessageCommand(commandCount, cmd)
	case "user_pull_message_command":
		return c.UserPullMessageCommand(commandCount, cmd)
	case "user_show_message_queue_command":
		return c.UserShowMessageQueueCommand(commandCount, cmd)
	case "admin_check_license":
		return c.AdminCheckLicenseCommand(commandCount, cmd)
	case "admin_check_provider_with_key":
		return c.CommonCheckProviderWithKeyCommand(commandCount, cmd)
	case "admin_check_provider_instance":
		return c.CommonCheckProviderConnectionCommand(commandCount, cmd)
	case "admin_show_fingerprint":
		return c.AdminShowFingerprintCommand(commandCount, cmd)
	case "admin_show_soft_fingerprint":
		return c.AdminShowSoftFingerprintCommand(commandCount, cmd)
	case "admin_show_license":
		return c.AdminShowLicenseCommand(commandCount, cmd)
	case "admin_show_user":
		return c.AdminShowUserCommand(commandCount, cmd)
	case "admin_show_role":
		return c.AdminShowRoleCommand(commandCount, cmd)
	case "admin_show_role_default_models":
		return c.AdminShowRoleDefaultModelsCommand(commandCount, cmd)
	case "admin_show_user_activity_command":
		return c.AdminShowUserActivityCommand(commandCount, cmd)
	case "admin_show_user_summary_command":
		return c.AdminShowUserSummaryCommand(commandCount, cmd)
	case "admin_show_user_dataset_command":
		return c.AdminShowUserDatasetCommand(commandCount, cmd)
	case "admin_show_user_storage_command":
		return c.AdminShowUserStorageCommand(commandCount, cmd)
	case "admin_show_user_quota_command":
		return c.AdminShowUserQuotaCommand(commandCount, cmd)
	case "admin_show_user_index_command":
		return c.AdminShowUserIndexCommand(commandCount, cmd)
	case "admin_show_user_permission_command":
		return c.AdminShowUserPermissionCommand(commandCount, cmd)
	case "admin_show_users_summary_command":
		return c.AdminShowUsersSummaryCommand(commandCount, cmd)
	case "admin_show_users_activity_command":
		return c.AdminShowUsersActivityCommand(commandCount, cmd)
	case "admin_show_users_plan_summary":
		return c.AdminShowUsersPlanSummaryCommand(commandCount, cmd)
	case "admin_show_users_plan_quota":
		return c.AdminShowUsersPlanQuotaCommand(commandCount, cmd)
	case "admin_stats_user":
		return c.AdminStatsUserCommand(commandCount, cmd)
	case "admin_stats_users":
		return c.AdminStatsUsersCommand(commandCount, cmd)
	case "admin_stats_summary":
		return c.AdminStatsSummaryCommand(commandCount, cmd)
	case "admin_list_users_command":
		return c.AdminListUsersCommand(commandCount, cmd)
	case "admin_list_users_condition_command":
		return c.AdminListUsersConditionCommand(commandCount, cmd)
	case "admin_show_quota_summary":
		return c.AdminShowQuotaSummaryCommand(commandCount, cmd)
	case "admin_show_tasks_summary":
		return c.AdminShowTasksSummaryCommand(commandCount, cmd)
	case "admin_show_data_summary":
		return c.AdminShowDataSummaryCommand(commandCount, cmd)
	case "admin_show_data_orphan":
		return c.AdminShowDataOrphanCommand(commandCount, cmd)
	case "admin_show_data_storage":
		return c.AdminShowDataStorageCommand(commandCount, cmd)
	case "admin_show_data_index":
		return c.AdminShowDataIndexCommand(commandCount, cmd)
	case "admin_purge_orphan_command":
		return c.AdminPurgeOrphanCommand(commandCount, cmd)
	case "admin_purge_user_command":
		return c.AdminPurgeUserCommand(commandCount, cmd)
	case "admin_purge_users_command":
		return c.AdminPurgeUsersCommand(commandCount, cmd)
	case "admin_list_user_ingestion_tasks":
		return c.AdminListUserIngestionTasksCommand(commandCount, cmd)
	case "admin_list_user_datasets":
		return c.AdminListUserDatasetsCommand(commandCount, cmd)
	case "admin_list_user_agents":
		return c.AdminListUserAgentsCommand(commandCount, cmd)
	case "admin_list_user_chats":
		return c.AdminListUserChatsCommand(commandCount, cmd)
	case "admin_list_user_searches":
		return c.AdminListUserSearchesCommand(commandCount, cmd)
	case "admin_list_user_models":
		return c.AdminListUserModelsCommand(commandCount, cmd)
	case "admin_list_user_files":
		return c.AdminListUserFilesCommand(commandCount, cmd)
	case "admin_list_user_keys":
		return c.AdminListUserKeysCommand(commandCount, cmd)
	case "admin_list_user_providers":
		return c.AdminListUserProvidersCommand(commandCount, cmd)
	case "admin_list_user_provider_instances":
		return c.AdminListUserProviderInstancesCommand(commandCount, cmd)
	case "admin_list_user_provider_instance_models":
		return c.AdminListUserProviderInstanceModelsCommand(commandCount, cmd)
	case "admin_list_user_default_models":
		return c.AdminListUserDefaultModelsCommand(commandCount, cmd)
	case "admin_list_user_operation_logs":
		return c.AdminListUserLogsCommand(commandCount, cmd)
	case "admin_stop_user_ingestion_tasks_command":
		return c.AdminStopUserIngestionTasksCommand(commandCount, cmd)
	case "admin_remove_user_ingestion_tasks_command":
		return c.AdminRemoveUserIngestionTasksCommand(commandCount, cmd)
	case "admin_add_provider":
		return c.AdminAddProviderCommand(commandCount, cmd)
	case "admin_add_model_instance":
		return c.AdminAddModelInstanceCommand(commandCount, cmd)
	case "admin_add_models":
		return c.AdminAddModelsCommand(commandCount, cmd)
	case "admin_delete_model_providers":
		return c.AdminDeleteProvidersCommand(commandCount, cmd)
	case "admin_delete_model_instance":
		return c.AdminDeleteInstancesCommand(commandCount, cmd)
	case "admin_delete_model":
		return c.AdminDeleteModelsCommand(commandCount, cmd)
	case "admin_delete_soft_fingerprint":
		return c.AdminDeleteSoftFingerprintCommand(commandCount, cmd)
	case "admin_enable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "enable")
	case "admin_disable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "disable")
	case "admin_show_admin_server":
		return c.CommonShowAdminServerCommand(commandCount, cmd)
	case "admin_show_api_server":
		return c.CommonShowAPIServerCommand(commandCount, cmd)
	case "admin_show_log_level":
		return c.AdminShowLogLevelCommand(commandCount, cmd)
	case "admin_show_hardware":
		return c.CommonShowHardwareCommand(commandCount, cmd)
	case "admin_list_api_servers":
		return c.CommonListAPIServersCommand(commandCount, cmd)
	case "api_add_api_server":
		return c.AddAPIServerCommand(commandCount, cmd)
	case "api_delete_api_server":
		return c.DeleteAPIServerCommand(commandCount, cmd)
	case "api_add_admin_server":
		return nil, fmt.Errorf("cannot add admin server in admin mode")
	case "api_delete_admin_server":
		return nil, fmt.Errorf("cannot delete admin server in admin mode")
	case "admin_save_config_command":
		return c.CommonSaveServerConfigCommand(commandCount, cmd)
	case "admin_use_api_server":
		return c.CommonUseAPIServerCommand(commandCount, cmd)
	case "admin_use_admin_server":
		return c.CommonUseAdminServerCommand(commandCount, cmd)
	case "admin_list_bucket_objects":
		return c.AdminListBucketObjects(commandCount, cmd)
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}
}
func (c *CLI) ExecuteUserCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	switch cmd.Type {
	case "api_register_user":
		return c.RegisterUser(commandCount, cmd)
	case "api_login_user":
		return c.LoginUserByCommand(commandCount, cmd)
	case "api_logout":
		return c.Logout()
	case "api_ping_server":
		return c.PingServerByCommand(commandCount, cmd)
	case "api_set_log_level":
		return c.APISetLogLevelCommand(commandCount, cmd)
	case "benchmark":
		return c.RunBenchmark(commandCount, cmd)
	case "api_list_datasets":
		return c.APIListDatasetsCommand(commandCount, cmd)
	case "api_list_dataset_documents":
		return c.APIListDatasetDocumentsCommand(commandCount, cmd)
	case "api_list_dataset_files":
		return c.APIListDatasetFilesCommand(commandCount, cmd)
	case "api_list_agents":
		return c.APIListAgentsCommand(commandCount, cmd)
	case "api_list_chats":
		return c.APIListChatsCommand(commandCount, cmd)
	case "api_list_searches":
		return c.APIListSearchesCommand(commandCount, cmd)
	case "api_list_memories":
		return c.APIListMemoriesCommand(commandCount, cmd)
	case "search_on_datasets":
		return c.SearchOnDatasets(commandCount, cmd)
	case "search_help":
		printSearchHelp()
		return nil, nil
	case "api_create_api_key":
		return c.APICreateAPIKeyCommand(commandCount, cmd)
	case "api_create_dataset":
		return c.APICreateDatasetCommand(commandCount, cmd)
	case "api_create_agent":
		return c.APICreateAgentCommand(commandCount, cmd)
	case "api_create_chat":
		return c.APICreateChatCommand(commandCount, cmd)
	case "api_create_search":
		return c.APICreateSearchCommand(commandCount, cmd)
	case "api_create_memory":
		return c.APICreateMemoryCommand(commandCount, cmd)
	case "api_list_api_keys":
		return c.APIListAPIKeysCommand(commandCount, cmd)
	case "api_delete_api_key":
		return c.APIDeleteAPIKeyCommand(commandCount, cmd)
	case "api_set_api_key":
		return c.APISetAPIKeyCommand(commandCount, cmd)
	case "api_set_variable":
		return c.APISetVariableCommand(commandCount, cmd)
	case "api_show_variable":
		return c.APIShowVariableCommand(commandCount, cmd)
	case "api_unset_api_key":
		return c.APIUnsetAPIKeyCommand(commandCount, cmd)
	case "api_show_version":
		return c.APIShowVersionCommand(commandCount, cmd)
	case "api_show_api_key":
		return c.APIShowAPIKeyCommand(commandCount, cmd)
	case "api_show_current":
		return c.CommonShowCurrentCommand(commandCount, cmd)
	case "api_list_available_providers":
		return c.CommonAvailableProvidersCommand(commandCount, cmd)
	case "api_show_provider":
		return c.CommonShowProviderCommand(commandCount, cmd)
	case "api_show_provider_instance":
		return c.CommonShowProviderInstanceCommand(commandCount, cmd)
	case "api_show_provider_instance_balance":
		return c.CommonShowProviderInstanceBalanceCommand(commandCount, cmd)
	case "api_show_provider_instance_task":
		return c.APIShowProviderInstanceTaskCommand(commandCount, cmd)
	case "api_show_provider_model":
		return c.CommonShowProviderModelCommand(commandCount, cmd)
	case "api_list_provider_models":
		return c.CommonListModelsCommand(commandCount, cmd)
	case "api_list_provider_instance_models":
		return c.CommonListInstanceModelsCommand(commandCount, cmd)
	case "api_list_provider_instance_models_sync":
		return c.CommonListInstanceModelsSyncCommand(commandCount, cmd)
	case "api_list_provider_instance_tasks":
		return c.APIListModelInstanceTasksCommand(commandCount, cmd)

	// Provider commands
	case "api_show_model":
		return c.CommonShowModelCommand(commandCount, cmd)
	case "api_list_all_models":
		return c.CommonListAllModels(commandCount, cmd)
	case "api_add_provider":
		return c.APIAddProviderCommand(commandCount, cmd)
	case "api_list_providers":
		return c.APIListProvidersCommand(commandCount, cmd)
	case "api_delete_provider":
		return c.APIDeleteProviderCommand(commandCount, cmd)
	case "api_delete_provider_instance":
		return c.APIDeleteProviderInstanceCommand(commandCount, cmd)
	case "api_drop_dataset":
		return c.APIDropDatasetCommand(commandCount, cmd)
	case "api_drop_chat":
		return c.APIDropChatCommand(commandCount, cmd)
	case "api_drop_search":
		return c.APIDropSearchCommand(commandCount, cmd)
	case "api_drop_memory":
		return c.APIDropMemoryCommand(commandCount, cmd)
	case "api_drop_agent":
		return c.APIDropAgentCommand(commandCount, cmd)
	case "api_add_provider_instance":
		return c.APIAddProviderInstanceCommand(commandCount, cmd)
	case "api_list_provider_instances":
		return c.CommonListProviderInstancesCommand(commandCount, cmd)
	case "api_alter_provider_instance":
		return c.CommonAlterProviderInstanceCommand(commandCount, cmd)
	case "api_delete_provider_instance_model":
		return c.APIDeleteProviderInstanceModelCommand(commandCount, cmd)
	case "enable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "enable")
	case "disable_model":
		return c.CommonEnableOrDisableModelCommand(cmd, "disable")
	case "api_add_custom_model":
		return c.APIAddCustomModelCommand(commandCount, cmd)
	case "api_chat_to_model":
		return c.APIChatToModelCommand(commandCount, cmd)
	case "api_openai_chat":
		return c.APIOpenaiChatCommand(commandCount, cmd)
	case "openai_chat_help":
		printOpenaiChatHelp()
		return nil, nil
	case "api_embed_user_text":
		return c.EmbedUserTextCommand(commandCount, cmd)
	case "api_rarank_user_document":
		return c.APIRerankUserDocumentCommand(commandCount, cmd)
	case "chat completions":
		return c.ChatCompletions(commandCount, cmd)
	case "chat completions help":
		printChatCompletionsHelp()
		return nil, nil
	case "tts_user_command":
		return c.APITTSUserCommand(commandCount, cmd)
	case "asr_user_command":
		return c.APIASRUserCommand(commandCount, cmd)
	case "ocr_user_command":
		return c.APIOCRUserCommand(commandCount, cmd)
	case "api_model_parse_file":
		return c.APIModelParseFileCommand(commandCount, cmd)
	case "check_provider_connection":
		return c.CommonCheckProviderConnectionCommand(commandCount, cmd)
	case "check_provider_with_key":
		return c.CommonCheckProviderWithKeyCommand(commandCount, cmd)
	case "api_use_model":
		return c.APIUseModelCommand(commandCount, cmd)
	case "api_use_api_server":
		return c.CommonUseAPIServerCommand(commandCount, cmd)
	case "api_use_admin_server":
		return c.CommonUseAdminServerCommand(commandCount, cmd)
	case "api_set_default_model":
		return c.APISetDefaultModelCommand(commandCount, cmd)
	case "api_reset_default_model":
		return c.APIResetDefaultModelCommand(commandCount, cmd)
	case "api_list_default_models":
		return c.APIListDefaultModelsCommand(commandCount, cmd)
	case "api_parse_documents":
		return c.APIParseDocumentsCommand(commandCount, cmd)
	case "api_start_ingestion":
		return c.APIStartIngestionCommand(commandCount, cmd)
	case "api_stop_ingestion":
		return c.APIStopIngestionCommand(commandCount, cmd)
	case "api_list_ingestion_tasks":
		return c.APIListIngestionTasks(commandCount, cmd)
	case "api_list_sync_logs":
		return c.APIListSyncLogsCommand(commandCount, cmd)
	case "api_remove_task":
		return c.APIRemoveTaskCommand(commandCount, cmd)
	case "user_parse_local_file_command":
		return c.APIParseLocalFileCommand(commandCount, cmd)
	case "api_show_admin_server":
		return c.CommonShowAdminServerCommand(commandCount, cmd)
	case "api_show_api_server":
		return c.CommonShowAPIServerCommand(commandCount, cmd)
	case "api_show_log_level":
		return c.APIShowLogLevelCommand(commandCount, cmd)
	case "api_show_hardware":
		return c.CommonShowHardwareCommand(commandCount, cmd)
	case "api_list_api_servers":
		return c.CommonListAPIServersCommand(commandCount, cmd)
	case "api_list_environments":
		return c.APIListEnvironmentsCommand(commandCount, cmd)
	case "api_list_variables":
		return c.APIListVariablesCommand(commandCount, cmd)
	case "api_add_api_server":
		return c.AddAPIServerCommand(commandCount, cmd)
	case "api_delete_api_server":
		return c.DeleteAPIServerCommand(commandCount, cmd)
	case "api_add_admin_server":
		return c.AddAdminServerCommand(commandCount, cmd)
	case "api_delete_admin_server":
		return c.DeleteAdminServerCommand(commandCount, cmd)
	case "api_save_config_command":
		return c.CommonSaveServerConfigCommand(commandCount, cmd)

	// File system commands
	case "file_system_command":
		return c.ExecuteFilesystemCommand(commandCount, cmd)

	// For debug
	case "dev_chunk":
		return c.DevChunkCommand(commandCount, cmd)
	case "dev_create_chunk_store":
		return c.DevCreateChunkStoreCommand(commandCount, cmd)
	case "dev_drop_chunk_store":
		return c.DevDropChunkStoreCommand(commandCount, cmd)
	case "dev_create_metadata_store":
		return c.DevCreateMetadataStoreCommand(commandCount, cmd)
	case "dev_drop_metadata_store":
		return c.DevDropMetadataStoreCommand(commandCount, cmd)
	case "dev_update_chunk":
		return c.DevUpdateChunkCommand(commandCount, cmd)
	case "dev_get_chunk":
		return c.DevGetChunkCommand(commandCount, cmd)
	case "dev_set_meta":
		return c.DevSetMetaCommand(commandCount, cmd)
	case "dev_delete_meta":
		return c.DevDeleteMetaCommand(commandCount, cmd)
	case "dev_rm_tags":
		return c.DevRmTagsCommand(commandCount, cmd)
	case "dev_remove_chunks":
		return c.DevRemoveChunksCommand(commandCount, cmd)
	case "dev_get_metadata":
		return c.DevGetMetadataCommand(commandCount, cmd)
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}

}
