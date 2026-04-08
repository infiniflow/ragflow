const webAPI = `/v1`;
const restAPIv1 = `/api/v1`;

export { restAPIv1, webAPI };

export default {
  // user
  login: `${webAPI}/user/login`,
  logout: `${webAPI}/user/logout`,
  register: `${webAPI}/user/register`,
  setting: `${webAPI}/user/setting`,
  user_info: `${webAPI}/user/info`,
  tenant_info: `${webAPI}/user/tenant_info`,
  set_tenant_info: `${webAPI}/user/set_tenant_info`,
  login_channels: `${webAPI}/user/login/channels`,
  login_channel: (channel: string) => `${webAPI}/user/login/${channel}`,

  // team
  addTenantUser: (tenantId: string) => `${webAPI}/tenant/${tenantId}/user`,
  listTenantUser: (tenantId: string) =>
    `${webAPI}/tenant/${tenantId}/user/list`,
  deleteTenantUser: (tenantId: string, userId: string) =>
    `${webAPI}/tenant/${tenantId}/user/${userId}`,
  listTenant: `${webAPI}/tenant/list`,
  agreeTenant: (tenantId: string) => `${webAPI}/tenant/agree/${tenantId}`,

  // llm model
  factories_list: `${webAPI}/llm/factories`,
  llm_list: `${webAPI}/llm/list`,
  my_llm: `${webAPI}/llm/my_llms`,
  set_api_key: `${webAPI}/llm/set_api_key`,
  add_llm: `${webAPI}/llm/add_llm`,
  delete_llm: `${webAPI}/llm/delete_llm`,
  enable_llm: `${webAPI}/llm/enable_llm`,
  deleteFactory: `${webAPI}/llm/delete_factory`,

  // data source
  dataSourceSet: `${webAPI}/connector/set`,
  dataSourceList: `${webAPI}/connector/list`,
  dataSourceDel: (id: string) => `${webAPI}/connector/${id}/rm`,
  dataSourceResume: (id: string) => `${webAPI}/connector/${id}/resume`,
  dataSourceRebuild: (id: string) => `${webAPI}/connector/${id}/rebuild`,
  dataSourceLogs: (id: string) => `${webAPI}/connector/${id}/logs`,
  dataSourceDetail: (id: string) => `${webAPI}/connector/${id}`,
  googleWebAuthStart: (type: 'google-drive' | 'gmail') =>
    `${webAPI}/connector/google/oauth/web/start?type=${type}`,
  googleWebAuthResult: (type: 'google-drive' | 'gmail') =>
    `${webAPI}/connector/google/oauth/web/result?type=${type}`,
  boxWebAuthStart: () => `${webAPI}/connector/box/oauth/web/start`,
  boxWebAuthResult: () => `${webAPI}/connector/box/oauth/web/result`,

  // plugin
  llm_tools: `${webAPI}/plugin/llm_tools`,

  chatsTranscriptions: `${restAPIv1}/chats/transcriptions`,

  // knowledge base

  check_embedding: `${webAPI}/kb/check_embedding`,
  kb_list: `${restAPIv1}/datasets`,
  create_kb: `${restAPIv1}/datasets`,
  update_kb: (datasetId: string) => `${restAPIv1}/datasets/${datasetId}`,
  rm_kb: `${restAPIv1}/datasets`,
  get_kb_detail: `${webAPI}/kb/detail`,
  getKnowledgeGraph: (knowledgeId: string) =>
    `${restAPIv1}/datasets/${knowledgeId}/knowledge_graph`,
  deleteKnowledgeGraph: (knowledgeId: string) =>
    `${restAPIv1}/datasets/${knowledgeId}/knowledge_graph`,
  getMeta: `${webAPI}/kb/get_meta`,
  getKnowledgeBasicInfo: `${webAPI}/kb/basic_info`,
  // data pipeline log
  fetchDataPipelineLog: `${webAPI}/kb/list_pipeline_logs`,
  get_pipeline_detail: `${webAPI}/kb/pipeline_log_detail`,
  fetchPipelineDatasetLogs: `${webAPI}/kb/list_pipeline_dataset_logs`,
  runGraphRag: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/run_graphrag`,
  traceGraphRag: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/trace_graphrag`,
  runRaptor: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/run_raptor`,
  traceRaptor: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/trace_raptor`,
  unbindPipelineTask: ({ kb_id, type }: { kb_id: string; type: string }) =>
    `${webAPI}/kb/unbind_task?kb_id=${kb_id}&pipeline_task_type=${type}`,
  pipelineRerun: `${webAPI}/canvas/rerun`,
  getMetaData: `${webAPI}/document/metadata/summary`,
  updateMetaData: `${webAPI}/document/metadata/update`,
  kbUpdateMetaData: `${webAPI}/kb/update_metadata_setting`,
  documentUpdateMetaData: `${webAPI}/document/update_metadata_setting`,

  // tags
  listTag: (knowledgeId: string) => `${webAPI}/kb/${knowledgeId}/tags`,
  listTagByKnowledgeIds: `${webAPI}/kb/tags`,
  removeTag: (knowledgeId: string) => `${webAPI}/kb/${knowledgeId}/rm_tags`,
  renameTag: (knowledgeId: string) => `${webAPI}/kb/${knowledgeId}/rename_tag`,

  // chunk
  chunk_list: `${webAPI}/chunk/list`,
  create_chunk: `${webAPI}/chunk/create`,
  set_chunk: `${webAPI}/chunk/set`,
  get_chunk: `${webAPI}/chunk/get`,
  switch_chunk: `${webAPI}/chunk/switch`,
  rm_chunk: `${webAPI}/chunk/rm`,
  retrieval_test: `${webAPI}/chunk/retrieval_test`,
  knowledge_graph: `${webAPI}/chunk/knowledge_graph`,

  // document
  get_document_list: `${webAPI}/document/list`,
  document_change_status: `${webAPI}/document/change_status`,
  document_rm: `${webAPI}/document/rm`,
  document_delete: `${webAPI}/api/document`,
  document_rename: `${webAPI}/document/rename`,
  document_create: `${webAPI}/document/create`,
  document_run: `${webAPI}/document/run`,
  document_change_parser: `${webAPI}/document/change_parser`,
  document_thumbnails: `${webAPI}/document/thumbnails`,
  get_document_file: `${webAPI}/document/get`,
  get_document_file_download: (docId: string) =>
    `${webAPI}/document/download/${docId}`,
  document_upload: `${webAPI}/document/upload`,
  web_crawl: `${webAPI}/document/web_crawl`,
  document_infos: `${webAPI}/document/infos`,
  upload_and_parse: `${webAPI}/document/upload_info`,
  parse: `${webAPI}/document/parse`,
  setMeta: `${webAPI}/document/set_meta`,
  get_dataset_filter: `${webAPI}/document/filter`,

  // chat
  createChat: `${restAPIv1}/chats`,
  listChats: `${restAPIv1}/chats`,
  getChat: (chatId: string) => `${restAPIv1}/chats/${chatId}`,
  updateChat: (chatId: string) => `${restAPIv1}/chats/${chatId}`,
  patchChat: (chatId: string) => `${restAPIv1}/chats/${chatId}`,
  deleteChat: (chatId: string) => `${restAPIv1}/chats/${chatId}`,
  bulkDeleteChats: `${restAPIv1}/chats`,
  createSession: (chatId: string) => `${restAPIv1}/chats/${chatId}/sessions`,
  listSessions: (chatId: string) => `${restAPIv1}/chats/${chatId}/sessions`,
  getSession: (chatId: string, sessionId: string) =>
    `${restAPIv1}/chats/${chatId}/sessions/${sessionId}`,
  updateSession: (chatId: string, sessionId: string) =>
    `${restAPIv1}/chats/${chatId}/sessions/${sessionId}`,
  removeSessions: (chatId: string) => `${restAPIv1}/chats/${chatId}/sessions`,
  deleteMessage: (chatId: string, sessionId: string, msgId: string) =>
    `${restAPIv1}/chats/${chatId}/sessions/${sessionId}/messages/${msgId}`,
  thumbup: (chatId: string, sessionId: string, msgId: string) =>
    `${restAPIv1}/chats/${chatId}/sessions/${sessionId}/messages/${msgId}/feedback`,
  completionUrl: (chatId: string, sessionId: string) =>
    `${restAPIv1}/chats/${chatId}/sessions/${sessionId}/completions`,
  chatsTts: `${restAPIv1}/chats/tts`,
  ask: `${restAPIv1}/chats/ask`,
  chatsMindmap: `${restAPIv1}/chats/mindmap`,
  chatsRelatedQuestions: `${restAPIv1}/chats/related_questions`,
  // chat for external
  createToken: `${webAPI}/api/new_token`,
  listToken: `${webAPI}/api/token_list`,
  removeToken: `${webAPI}/api/rm`,
  getStats: `${webAPI}/api/stats`,

  // next chat
  fetchExternalChatInfo: (id: string) => `${restAPIv1}/chatbots/${id}/info`,

  // file manager
  listFile: `${restAPIv1}/files`,
  uploadFile: `${restAPIv1}/files`,
  removeFile: `${restAPIv1}/files`,
  getAllParentFolder: `${restAPIv1}/files`,
  createFolder: `${restAPIv1}/files`,
  connectFileToKnowledge: `${webAPI}/file2document/convert`,
  getFile: `${restAPIv1}/files`,
  moveFile: `${restAPIv1}/files/move`,

  // system
  getSystemVersion: `${restAPIv1}/system/version`,
  getSystemTokenList: `${webAPI}/system/token_list`,
  createSystemToken: `${webAPI}/system/new_token`,
  removeSystemToken: `${webAPI}/system/token`,
  getSystemConfig: `${webAPI}/system/config`,
  setLangfuseConfig: `${webAPI}/langfuse/api_key`,

  // flow
  listTemplates: `${webAPI}/canvas/templates`,
  listCanvas: `${webAPI}/canvas/list`,
  getCanvas: `${webAPI}/canvas/get`,
  getCanvasSSE: (canvasId: string) => `${webAPI}/canvas/getsse/${canvasId}`,
  removeCanvas: `${webAPI}/canvas/rm`,
  setCanvas: `${webAPI}/canvas/set`,
  settingCanvas: `${webAPI}/canvas/setting`,
  getListVersion: `${webAPI}/canvas/getlistversion`,
  getVersion: `${webAPI}/canvas/getversion`,
  resetCanvas: `${webAPI}/canvas/reset`,
  runCanvas: `${webAPI}/canvas/completion`,
  testDbConnect: `${webAPI}/canvas/test_db_connect`,
  getInputElements: `${webAPI}/canvas/input_elements`,
  debug: `${webAPI}/canvas/debug`,
  uploadCanvasFile: `${webAPI}/canvas/upload`,
  trace: `${webAPI}/canvas/trace`,
  cancelCanvas: (taskId: string) => `${webAPI}/canvas/cancel/${taskId}`, // cancel conversation
  // agent
  inputForm: `${webAPI}/canvas/input_form`,
  fetchVersionList: (id: string) => `${webAPI}/canvas/getlistversion/${id}`,
  fetchVersion: (id: string) => `${webAPI}/canvas/getversion/${id}`,
  fetchCanvas: (id: string) => `${webAPI}/canvas/get/${id}`,
  fetchAgentAvatar: (id: string) => `${webAPI}/canvas/getsse/${id}`,
  uploadAgentFile: (id?: string) => `${webAPI}/canvas/upload/${id}`,
  fetchAgentLogs: (canvasId: string) => `${webAPI}/canvas/${canvasId}/sessions`,
  fetchAgentLogsById: (canvasId: string, sessionId: string) =>
    `${webAPI}/canvas/${canvasId}/sessions/${sessionId}`,
  fetchExternalAgentInputs: (canvasId: string) =>
    `${restAPIv1}/agentbots/${canvasId}/inputs`,
  prompt: `${webAPI}/canvas/prompts`,
  cancelDataflow: (id: string) => `${webAPI}/canvas/cancel/${id}`,
  downloadFile: `${webAPI}/canvas/download`,
  testWebhook: (id: string) => `${restAPIv1}/webhook_test/${id}`,
  fetchWebhookTrace: (id: string) => `${restAPIv1}/webhook_trace/${id}`,

  // explore

  runCanvasExplore: (canvasId: string) =>
    `${webAPI}/canvas/${canvasId}/completion`,

  // mcp server
  listMcpServer: `${webAPI}/mcp_server/list`,
  getMcpServer: `${webAPI}/mcp_server/detail`,
  createMcpServer: `${webAPI}/mcp_server/create`,
  updateMcpServer: `${webAPI}/mcp_server/update`,
  deleteMcpServer: `${webAPI}/mcp_server/rm`,
  importMcpServer: `${webAPI}/mcp_server/import`,
  exportMcpServer: `${webAPI}/mcp_server/export`,
  listMcpServerTools: `${webAPI}/mcp_server/list_tools`,
  testMcpServerTool: `${webAPI}/mcp_server/test_tool`,
  cacheMcpServerTool: `${webAPI}/mcp_server/cache_tools`,
  testMcpServer: `${webAPI}/mcp_server/test_mcp`,

  // next-search
  createSearch: `${restAPIv1}/searches`,
  getSearchList: `${restAPIv1}/searches`,
  deleteSearch: (params: { search_id: string }) =>
    `${restAPIv1}/searches/${params.search_id}`,
  getSearchDetail: (params: { search_id: string }) =>
    `${restAPIv1}/searches/${params.search_id}`,
  getSearchDetailShare: `${restAPIv1}/searchbots/detail`,
  updateSearchSetting: (params: { search_id: string }) =>
    `${restAPIv1}/searches/${params.search_id}`,
  askShare: `${restAPIv1}/searchbots/ask`,
  mindmapShare: `${restAPIv1}/searchbots/mindmap`,
  getRelatedQuestionsShare: `${restAPIv1}/searchbots/related_questions`,
  retrievalTestShare: `${restAPIv1}/searchbots/retrieval_test`,

  // memory
  createMemory: `${restAPIv1}/memories`,
  getMemoryList: `${restAPIv1}/memories`,
  getMemoryConfig: (id: string) => `${restAPIv1}/memories/${id}/config`,
  deleteMemory: (id: string) => `${restAPIv1}/memories/${id}`,
  getMemoryDetail: (id: string) => `${restAPIv1}/memories/${id}`,
  updateMemorySetting: (id: string) => `${restAPIv1}/memories/${id}`,
  deleteMemoryMessage: (data: { memory_id: string; message_id: string }) =>
    `${restAPIv1}/messages/${data.memory_id}:${data.message_id}`,
  getMessageContent: (data: { memory_id: string; message_id: string }) =>
    `${restAPIv1}/messages/${data.memory_id}:${data.message_id}/content`,
  updateMessageState: (data: { memory_id: string; message_id: string }) =>
    `${restAPIv1}/messages/${data.memory_id}:${data.message_id}`,

  // data pipeline
  fetchDataflow: (id: string) => `${webAPI}/dataflow/get/${id}`,
  setDataflow: `${webAPI}/dataflow/set`,
  removeDataflow: `${webAPI}/dataflow/rm`,
  listDataflow: `${webAPI}/dataflow/list`,
  runDataflow: `${webAPI}/dataflow/run`,

  // admin
  adminLogin: `${restAPIv1}/admin/login`,
  adminLogout: `${restAPIv1}/admin/logout`,
  adminListUsers: `${restAPIv1}/admin/users`,
  adminCreateUser: `${restAPIv1}/admin/users`,
  adminSetSuperuser: (username: string) =>
    `${restAPIv1}/admin/users/${username}/admin`,
  adminGetUserDetails: (username: string) =>
    `${restAPIv1}/admin/users/${username}`,
  adminUpdateUserStatus: (username: string) =>
    `${restAPIv1}/admin/users/${username}/activate`,
  adminUpdateUserPassword: (username: string) =>
    `${restAPIv1}/admin/users/${username}/password`,
  adminDeleteUser: (username: string) => `${restAPIv1}/admin/users/${username}`,
  adminListUserDatasets: (username: string) =>
    `${restAPIv1}/admin/users/${username}/datasets`,
  adminListUserAgents: (username: string) =>
    `${restAPIv1}/admin/users/${username}/agents`,

  adminListServices: `${restAPIv1}/admin/services`,
  adminShowServiceDetails: (serviceId: string) =>
    `${restAPIv1}/admin/services/${serviceId}`,

  adminListRoles: `${restAPIv1}/admin/roles`,
  adminListRolesWithPermission: `${restAPIv1}/admin/roles_with_permission`,
  adminGetRolePermissions: (roleName: string) =>
    `${restAPIv1}/admin/roles/${roleName}/permissions`,
  adminAssignRolePermissions: (roleName: string) =>
    `${restAPIv1}/admin/roles/${roleName}/permission`,
  adminRevokeRolePermissions: (roleName: string) =>
    `${restAPIv1}/admin/roles/${roleName}/permission`,
  adminCreateRole: `${restAPIv1}/admin/roles`,
  adminDeleteRole: (roleName: string) => `${restAPIv1}/admin/roles/${roleName}`,
  adminUpdateRoleDescription: (roleName: string) =>
    `${restAPIv1}/admin/roles/${roleName}`,

  adminUpdateUserRole: (username: string) =>
    `${restAPIv1}/admin/users/${username}/role`,
  adminGetUserPermissions: (username: string) =>
    `${restAPIv1}/admin/users/${username}/permissions`,

  adminListResources: `${restAPIv1}/admin/roles/resource`,

  adminListWhitelist: `${restAPIv1}/admin/whitelist`,
  adminCreateWhitelistEntry: `${restAPIv1}/admin/whitelist/add`,
  adminUpdateWhitelistEntry: (id: number) =>
    `${restAPIv1}/admin/whitelist/${id}`,
  adminDeleteWhitelistEntry: (email: string) =>
    `${restAPIv1}/admin/whitelist/${email}`,
  adminImportWhitelist: `${restAPIv1}/admin/whitelist/batch`,

  adminGetSystemVersion: `${restAPIv1}/admin/version`,

  // Sandbox settings
  adminListSandboxProviders: `${restAPIv1}/admin/sandbox/providers`,
  adminGetSandboxProviderSchema: (providerId: string) =>
    `${restAPIv1}/admin/sandbox/providers/${providerId}/schema`,
  adminGetSandboxConfig: `${restAPIv1}/admin/sandbox/config`,
  adminSetSandboxConfig: `${restAPIv1}/admin/sandbox/config`,
  adminTestSandboxConnection: `${restAPIv1}/admin/sandbox/test`,
};
