let api_host = `/v1`;
const ExternalApi = `/api`;

export { api_host };

export default {
  // user
  login: `${api_host}/user/login`,
  logout: `${api_host}/user/logout`,
  register: `${api_host}/user/register`,
  setting: `${api_host}/user/setting`,
  user_info: `${api_host}/user/info`,
  tenant_info: `${api_host}/user/tenant_info`,
  set_tenant_info: `${api_host}/user/set_tenant_info`,
  login_channels: `${api_host}/user/login/channels`,
  login_channel: (channel: string) => `${api_host}/user/login/${channel}`,

  // team
  addTenantUser: (tenantId: string) => `${api_host}/tenant/${tenantId}/user`,
  listTenantUser: (tenantId: string) =>
    `${api_host}/tenant/${tenantId}/user/list`,
  deleteTenantUser: (tenantId: string, userId: string) =>
    `${api_host}/tenant/${tenantId}/user/${userId}`,
  listTenant: `${api_host}/tenant/list`,
  agreeTenant: (tenantId: string) => `${api_host}/tenant/agree/${tenantId}`,

  // llm model
  factories_list: `${api_host}/llm/factories`,
  llm_list: `${api_host}/llm/list`,
  my_llm: `${api_host}/llm/my_llms`,
  set_api_key: `${api_host}/llm/set_api_key`,
  add_llm: `${api_host}/llm/add_llm`,
  delete_llm: `${api_host}/llm/delete_llm`,
  enable_llm: `${api_host}/llm/enable_llm`,
  deleteFactory: `${api_host}/llm/delete_factory`,

  // data source
  dataSourceSet: `${api_host}/connector/set`,
  dataSourceList: `${api_host}/connector/list`,
  dataSourceDel: (id: string) => `${api_host}/connector/${id}/rm`,
  dataSourceResume: (id: string) => `${api_host}/connector/${id}/resume`,
  dataSourceRebuild: (id: string) => `${api_host}/connector/${id}/rebuild`,
  dataSourceLogs: (id: string) => `${api_host}/connector/${id}/logs`,
  dataSourceDetail: (id: string) => `${api_host}/connector/${id}`,
  googleWebAuthStart: (type: 'google-drive' | 'gmail') =>
    `${api_host}/connector/google/oauth/web/start?type=${type}`,
  googleWebAuthResult: (type: 'google-drive' | 'gmail') =>
    `${api_host}/connector/google/oauth/web/result?type=${type}`,
  boxWebAuthStart: () => `${api_host}/connector/box/oauth/web/start`,
  boxWebAuthResult: () => `${api_host}/connector/box/oauth/web/result`,

  // plugin
  llm_tools: `${api_host}/plugin/llm_tools`,

  sequence2txt: `${api_host}/conversation/sequence2txt`,

  // knowledge base

  check_embedding: `${api_host}/kb/check_embedding`,
  kb_list: `${api_host}/kb/list`,
  create_kb: `${api_host}/kb/create`,
  update_kb: `${api_host}/kb/update`,
  rm_kb: `${api_host}/kb/rm`,
  get_kb_detail: `${api_host}/kb/detail`,
  getKnowledgeGraph: (knowledgeId: string) =>
    `${api_host}/kb/${knowledgeId}/knowledge_graph`,
  getMeta: `${api_host}/kb/get_meta`,
  getKnowledgeBasicInfo: `${api_host}/kb/basic_info`,
  // data pipeline log
  fetchDataPipelineLog: `${api_host}/kb/list_pipeline_logs`,
  get_pipeline_detail: `${api_host}/kb/pipeline_log_detail`,
  fetchPipelineDatasetLogs: `${api_host}/kb/list_pipeline_dataset_logs`,
  runGraphRag: `${api_host}/kb/run_graphrag`,
  traceGraphRag: `${api_host}/kb/trace_graphrag`,
  runRaptor: `${api_host}/kb/run_raptor`,
  traceRaptor: `${api_host}/kb/trace_raptor`,
  unbindPipelineTask: ({ kb_id, type }: { kb_id: string; type: string }) =>
    `${api_host}/kb/unbind_task?kb_id=${kb_id}&pipeline_task_type=${type}`,
  pipelineRerun: `${api_host}/canvas/rerun`,
  getMetaData: `${api_host}/document/metadata/summary`,
  updateMetaData: `${api_host}/document/metadata/update`,
  kbUpdateMetaData: `${api_host}/kb/update_metadata_setting`,
  documentUpdateMetaData: `${api_host}/document/update_metadata_setting`,

  // tags
  listTag: (knowledgeId: string) => `${api_host}/kb/${knowledgeId}/tags`,
  listTagByKnowledgeIds: `${api_host}/kb/tags`,
  removeTag: (knowledgeId: string) => `${api_host}/kb/${knowledgeId}/rm_tags`,
  renameTag: (knowledgeId: string) =>
    `${api_host}/kb/${knowledgeId}/rename_tag`,

  // chunk
  chunk_list: `${api_host}/chunk/list`,
  create_chunk: `${api_host}/chunk/create`,
  set_chunk: `${api_host}/chunk/set`,
  get_chunk: `${api_host}/chunk/get`,
  switch_chunk: `${api_host}/chunk/switch`,
  rm_chunk: `${api_host}/chunk/rm`,
  retrieval_test: `${api_host}/chunk/retrieval_test`,
  knowledge_graph: `${api_host}/chunk/knowledge_graph`,

  // document
  get_document_list: `${api_host}/document/list`,
  document_change_status: `${api_host}/document/change_status`,
  document_rm: `${api_host}/document/rm`,
  document_delete: `${api_host}/api/document`,
  document_rename: `${api_host}/document/rename`,
  document_create: `${api_host}/document/create`,
  document_run: `${api_host}/document/run`,
  document_change_parser: `${api_host}/document/change_parser`,
  document_thumbnails: `${api_host}/document/thumbnails`,
  get_document_file: `${api_host}/document/get`,
  get_document_file_download: (docId: string) =>
    `${api_host}/document/download/${docId}`,
  document_upload: `${api_host}/document/upload`,
  web_crawl: `${api_host}/document/web_crawl`,
  document_infos: `${api_host}/document/infos`,
  upload_and_parse: `${api_host}/document/upload_info`,
  parse: `${api_host}/document/parse`,
  setMeta: `${api_host}/document/set_meta`,
  get_dataset_filter: `${api_host}/document/filter`,

  // chat
  setDialog: `${api_host}/dialog/set`,
  getDialog: `${api_host}/dialog/get`,
  removeDialog: `${api_host}/dialog/rm`,
  listDialog: `${api_host}/dialog/list`,
  setConversation: `${api_host}/conversation/set`,
  getConversation: `${api_host}/conversation/get`,
  getConversationSSE: (dialogId: string) =>
    `${api_host}/conversation/getsse/${dialogId}`,
  listConversation: `${api_host}/conversation/list`,
  removeConversation: `${api_host}/conversation/rm`,
  completeConversation: `${api_host}/conversation/completion`,
  deleteMessage: `${api_host}/conversation/delete_msg`,
  thumbup: `${api_host}/conversation/thumbup`,
  tts: `${api_host}/conversation/tts`,
  ask: `${api_host}/conversation/ask`,
  mindmap: `${api_host}/conversation/mindmap`,
  getRelatedQuestions: `${api_host}/conversation/related_questions`,
  // chat for external
  createToken: `${api_host}/api/new_token`,
  listToken: `${api_host}/api/token_list`,
  removeToken: `${api_host}/api/rm`,
  getStats: `${api_host}/api/stats`,
  createExternalConversation: `${api_host}/api/new_conversation`,
  getExternalConversation: `${api_host}/api/conversation`,
  completeExternalConversation: `${api_host}/api/completion`,
  uploadAndParseExternal: `${api_host}/api/document/upload_and_parse`,

  // next chat
  listNextDialog: `${api_host}/dialog/next`,
  fetchExternalChatInfo: (id: string) =>
    `${ExternalApi}${api_host}/chatbots/${id}/info`,

  // file manager
  listFile: `${api_host}/file/list`,
  uploadFile: `${api_host}/file/upload`,
  removeFile: `${api_host}/file/rm`,
  renameFile: `${api_host}/file/rename`,
  getAllParentFolder: `${api_host}/file/all_parent_folder`,
  createFolder: `${api_host}/file/create`,
  connectFileToKnowledge: `${api_host}/file2document/convert`,
  getFile: `${api_host}/file/get`,
  moveFile: `${api_host}/file/mv`,

  // system
  getSystemVersion: `${api_host}/system/version`,
  getSystemStatus: `${api_host}/system/status`,
  getSystemTokenList: `${api_host}/system/token_list`,
  createSystemToken: `${api_host}/system/new_token`,
  listSystemToken: `${api_host}/system/token_list`,
  removeSystemToken: `${api_host}/system/token`,
  getSystemConfig: `${api_host}/system/config`,
  setLangfuseConfig: `${api_host}/langfuse/api_key`,

  // flow
  listTemplates: `${api_host}/canvas/templates`,
  listCanvas: `${api_host}/canvas/list`,
  getCanvas: `${api_host}/canvas/get`,
  getCanvasSSE: (canvasId: string) => `${api_host}/canvas/getsse/${canvasId}`,
  removeCanvas: `${api_host}/canvas/rm`,
  setCanvas: `${api_host}/canvas/set`,
  settingCanvas: `${api_host}/canvas/setting`,
  getListVersion: `${api_host}/canvas/getlistversion`,
  getVersion: `${api_host}/canvas/getversion`,
  resetCanvas: `${api_host}/canvas/reset`,
  runCanvas: `${api_host}/canvas/completion`,
  testDbConnect: `${api_host}/canvas/test_db_connect`,
  getInputElements: `${api_host}/canvas/input_elements`,
  debug: `${api_host}/canvas/debug`,
  uploadCanvasFile: `${api_host}/canvas/upload`,
  trace: `${api_host}/canvas/trace`,
  cancelCanvas: (taskId: string) => `${api_host}/canvas/cancel/${taskId}`, // cancel conversation
  // agent
  inputForm: `${api_host}/canvas/input_form`,
  fetchVersionList: (id: string) => `${api_host}/canvas/getlistversion/${id}`,
  fetchVersion: (id: string) => `${api_host}/canvas/getversion/${id}`,
  fetchCanvas: (id: string) => `${api_host}/canvas/get/${id}`,
  fetchAgentAvatar: (id: string) => `${api_host}/canvas/getsse/${id}`,
  uploadAgentFile: (id?: string) => `${api_host}/canvas/upload/${id}`,
  fetchAgentLogs: (canvasId: string) =>
    `${api_host}/canvas/${canvasId}/sessions`,
  fetchExternalAgentInputs: (canvasId: string) =>
    `${ExternalApi}${api_host}/agentbots/${canvasId}/inputs`,
  prompt: `${api_host}/canvas/prompts`,
  cancelDataflow: (id: string) => `${api_host}/canvas/cancel/${id}`,
  downloadFile: `${api_host}/canvas/download`,
  testWebhook: (id: string) => `${ExternalApi}${api_host}/webhook_test/${id}`,
  fetchWebhookTrace: (id: string) =>
    `${ExternalApi}${api_host}/webhook_trace/${id}`,

  // mcp server
  listMcpServer: `${api_host}/mcp_server/list`,
  getMcpServer: `${api_host}/mcp_server/detail`,
  createMcpServer: `${api_host}/mcp_server/create`,
  updateMcpServer: `${api_host}/mcp_server/update`,
  deleteMcpServer: `${api_host}/mcp_server/rm`,
  importMcpServer: `${api_host}/mcp_server/import`,
  exportMcpServer: `${api_host}/mcp_server/export`,
  listMcpServerTools: `${api_host}/mcp_server/list_tools`,
  testMcpServerTool: `${api_host}/mcp_server/test_tool`,
  cacheMcpServerTool: `${api_host}/mcp_server/cache_tools`,
  testMcpServer: `${api_host}/mcp_server/test_mcp`,

  // next-search
  createSearch: `${api_host}/search/create`,
  getSearchList: `${api_host}/search/list`,
  deleteSearch: `${api_host}/search/rm`,
  getSearchDetail: `${api_host}/search/detail`,
  getSearchDetailShare: `${ExternalApi}${api_host}/searchbots/detail`,
  updateSearchSetting: `${api_host}/search/update`,
  askShare: `${ExternalApi}${api_host}/searchbots/ask`,
  mindmapShare: `${ExternalApi}${api_host}/searchbots/mindmap`,
  getRelatedQuestionsShare: `${ExternalApi}${api_host}/searchbots/related_questions`,
  retrievalTestShare: `${ExternalApi}${api_host}/searchbots/retrieval_test`,

  // memory
  createMemory: `${ExternalApi}${api_host}/memories`,
  getMemoryList: `${ExternalApi}${api_host}/memories`,
  getMemoryConfig: (id: string) =>
    `${ExternalApi}${api_host}/memories/${id}/config`,
  deleteMemory: (id: string) => `${ExternalApi}${api_host}/memories/${id}`,
  getMemoryDetail: (id: string) => `${ExternalApi}${api_host}/memories/${id}`,
  updateMemorySetting: (id: string) =>
    `${ExternalApi}${api_host}/memories/${id}`,
  deleteMemoryMessage: (data: { memory_id: string; message_id: string }) =>
    `${ExternalApi}${api_host}/messages/${data.memory_id}:${data.message_id}`,
  getMessageContent: (data: { memory_id: string; message_id: string }) =>
    `${ExternalApi}${api_host}/messages/${data.memory_id}:${data.message_id}/content`,
  updateMessageState: (data: { memory_id: string; message_id: string }) =>
    `${ExternalApi}${api_host}/messages/${data.memory_id}:${data.message_id}`,

  // data pipeline
  fetchDataflow: (id: string) => `${api_host}/dataflow/get/${id}`,
  setDataflow: `${api_host}/dataflow/set`,
  removeDataflow: `${api_host}/dataflow/rm`,
  listDataflow: `${api_host}/dataflow/list`,
  runDataflow: `${api_host}/dataflow/run`,

  // admin
  adminLogin: `${ExternalApi}${api_host}/admin/login`,
  adminLogout: `${ExternalApi}${api_host}/admin/logout`,
  adminListUsers: `${ExternalApi}${api_host}/admin/users`,
  adminCreateUser: `${ExternalApi}${api_host}/admin/users`,
  adminSetSuperuser: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}/admin`,
  adminGetUserDetails: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}`,
  adminUpdateUserStatus: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}/activate`,
  adminUpdateUserPassword: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}/password`,
  adminDeleteUser: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}`,
  adminListUserDatasets: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}/datasets`,
  adminListUserAgents: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}/agents`,

  adminListServices: `${ExternalApi}${api_host}/admin/services`,
  adminShowServiceDetails: (serviceId: string) =>
    `${ExternalApi}${api_host}/admin/services/${serviceId}`,

  adminListRoles: `${ExternalApi}${api_host}/admin/roles`,
  adminListRolesWithPermission: `${ExternalApi}${api_host}/admin/roles_with_permission`,
  adminGetRolePermissions: (roleName: string) =>
    `${ExternalApi}${api_host}/admin/roles/${roleName}/permissions`,
  adminAssignRolePermissions: (roleName: string) =>
    `${ExternalApi}${api_host}/admin/roles/${roleName}/permission`,
  adminRevokeRolePermissions: (roleName: string) =>
    `${ExternalApi}${api_host}/admin/roles/${roleName}/permission`,
  adminCreateRole: `${ExternalApi}${api_host}/admin/roles`,
  adminDeleteRole: (roleName: string) =>
    `${ExternalApi}${api_host}/admin/roles/${roleName}`,
  adminUpdateRoleDescription: (roleName: string) =>
    `${ExternalApi}${api_host}/admin/roles/${roleName}`,

  adminUpdateUserRole: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}/role`,
  adminGetUserPermissions: (username: string) =>
    `${ExternalApi}${api_host}/admin/users/${username}/permissions`,

  adminListResources: `${ExternalApi}${api_host}/admin/roles/resource`,

  adminListWhitelist: `${ExternalApi}${api_host}/admin/whitelist`,
  adminCreateWhitelistEntry: `${ExternalApi}${api_host}/admin/whitelist/add`,
  adminUpdateWhitelistEntry: (id: number) =>
    `${ExternalApi}${api_host}/admin/whitelist/${id}`,
  adminDeleteWhitelistEntry: (email: string) =>
    `${ExternalApi}${api_host}/admin/whitelist/${email}`,
  adminImportWhitelist: `${ExternalApi}${api_host}/admin/whitelist/batch`,

  adminGetSystemVersion: `${ExternalApi}${api_host}/admin/version`,

  // Sandbox settings
  adminListSandboxProviders: `${ExternalApi}${api_host}/admin/sandbox/providers`,
  adminGetSandboxProviderSchema: (providerId: string) =>
    `${ExternalApi}${api_host}/admin/sandbox/providers/${providerId}/schema`,
  adminGetSandboxConfig: `${ExternalApi}${api_host}/admin/sandbox/config`,
  adminSetSandboxConfig: `${ExternalApi}${api_host}/admin/sandbox/config`,
  adminTestSandboxConnection: `${ExternalApi}${api_host}/admin/sandbox/test`,
};
