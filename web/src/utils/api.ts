const webAPI = `/v1`;
const restAPIv1 = `/api/v1`;

export { restAPIv1, webAPI };

export default {
  // user
  login: `${webAPI}/user/login`,
  logout: `${webAPI}/user/logout`,
  register: `${webAPI}/user/register`,
  setting: `${webAPI}/user/setting`,
  userInfo: `${webAPI}/user/info`,
  tenantInfo: `${webAPI}/user/tenant_info`,
  setTenantInfo: `${webAPI}/user/set_tenant_info`,
  loginChannels: `${webAPI}/user/login/channels`,
  loginChannel: (channel: string) => `${webAPI}/user/login/${channel}`,

  // team
  addTenantUser: (tenantId: string) => `${restAPIv1}/tenants/${tenantId}/users`,
  listTenantUser: (tenantId: string) =>
    `${restAPIv1}/tenants/${tenantId}/users`,
  deleteTenantUser: (tenantId: string) =>
    `${restAPIv1}/tenants/${tenantId}/users`,
  listTenant: `${restAPIv1}/tenants`,
  agreeTenant: (tenantId: string) => `${restAPIv1}/tenants/${tenantId}`,

  // llm model
  factoriesList: `${webAPI}/llm/factories`,
  llmList: `${webAPI}/llm/list`,
  myLlm: `${webAPI}/llm/my_llms`,
  setApiKey: `${webAPI}/llm/set_api_key`,
  addLlm: `${webAPI}/llm/add_llm`,
  deleteLlm: `${webAPI}/llm/delete_llm`,
  enableLlm: `${webAPI}/llm/enable_llm`,
  deleteFactory: `${webAPI}/llm/delete_factory`,

  // data source
  dataSourceUpdate: (id: string) => `${restAPIv1}/connectors/${id}`,
  dataSourceSet: `${restAPIv1}/connectors`,
  dataSourceList: `${restAPIv1}/connectors`,
  dataSourceDel: (id: string) => `${restAPIv1}/connectors/${id}`,
  dataSourceResume: (id: string) => `${restAPIv1}/connectors/${id}/resume`,
  dataSourceRebuild: (id: string) => `${restAPIv1}/connectors/${id}/rebuild`,
  dataSourceLogs: (id: string) => `${restAPIv1}/connectors/${id}/logs`,
  dataSourceDetail: (id: string) => `${restAPIv1}/connectors/${id}`,
  googleWebAuthStart: (type: 'google-drive' | 'gmail') =>
    `${restAPIv1}/connectors/google/oauth/web/start?type=${type}`,
  googleWebAuthResult: (type: 'google-drive' | 'gmail') =>
    `${restAPIv1}/connectors/google/oauth/web/result?type=${type}`,
  boxWebAuthStart: () => `${restAPIv1}/connectors/box/oauth/web/start`,
  boxWebAuthResult: () => `${restAPIv1}/connectors/box/oauth/web/result`,

  // plugin
  llmTools: `${webAPI}/plugin/llm_tools`,

  chatsTranscriptions: `${restAPIv1}/chat/audio/transcription`,

  // knowledge base

  checkEmbedding: `${webAPI}/kb/check_embedding`,
  kbList: `${restAPIv1}/datasets`,
  createKb: `${restAPIv1}/datasets`,
  updateKb: (datasetId: string) => `${restAPIv1}/datasets/${datasetId}`,
  rmKb: `${restAPIv1}/datasets`,
  getKbDetail: `${webAPI}/kb/detail`,
  getKnowledgeGraph: (knowledgeId: string) =>
    `${restAPIv1}/datasets/${knowledgeId}/knowledge_graph`,
  deleteKnowledgeGraph: (knowledgeId: string) =>
    `${restAPIv1}/datasets/${knowledgeId}/knowledge_graph`,
  getMeta: `${webAPI}/kb/get_meta`,
  getKnowledgeBasicInfo: `${webAPI}/kb/basic_info`,
  // data pipeline log
  fetchDataPipelineLog: `${webAPI}/kb/list_pipeline_logs`,
  getPipelineDetail: `${webAPI}/kb/pipeline_log_detail`,
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
  getMetaData: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/metadata/summary`,
  updateDocumentsMetadata: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents/metadatas`,
  kbUpdateMetaData: `${webAPI}/kb/update_metadata_setting`,
  documentUpdateMetaDataConfig: (datasetId: string, documentId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents/${documentId}/metadata/config`,

  // tags
  listTag: (knowledgeId: string) => `${webAPI}/kb/${knowledgeId}/tags`,
  listTagByKnowledgeIds: `${webAPI}/kb/tags`,
  removeTag: (knowledgeId: string) => `${webAPI}/kb/${knowledgeId}/rm_tags`,
  renameTag: (knowledgeId: string) => `${webAPI}/kb/${knowledgeId}/rename_tag`,

  // chunk
  chunkList: `${webAPI}/chunk/list`,
  createChunk: `${webAPI}/chunk/create`,
  setChunk: `${webAPI}/chunk/set`,
  getChunk: `${webAPI}/chunk/get`,
  switchChunk: `${webAPI}/chunk/switch`,
  rmChunk: `${webAPI}/chunk/rm`,
  retrievalTest: `${webAPI}/chunk/retrieval_test`,
  knowledgeGraph: `${webAPI}/chunk/knowledge_graph`,

  // document
  getDocumentList: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents`,
  documentChangeStatus: `${webAPI}/document/change_status`,
  documentDelete: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents`,
  documentRename: (datasetId: string, documentId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents/${documentId}`,
  documentCreate: `${webAPI}/document/create`,
  documentRun: `${webAPI}/document/run`,
  documentChangeParser: `${webAPI}/document/change_parser`,
  documentUpdate: (datasetId: string, documentId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents/${documentId}`,
  documentThumbnails: `${webAPI}/document/thumbnails`,
  getDocumentFile: `${webAPI}/document/get`,
  getDocumentFileDownload: (docId: string) =>
    `${webAPI}/document/download/${docId}`,
  documentUpload: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents`,
  webCrawl: `${webAPI}/document/web_crawl`,
  uploadAndParse: `${webAPI}/document/upload_info`,
  setMeta: `${webAPI}/document/set_meta`,
  getDatasetFilter: (datasetId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents?type=filter`,

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
  completionUrl: `${restAPIv1}/chat/completions`,
  chatsTts: `${restAPIv1}/chat/audio/speech`,
  searchCompletion: (searchId: string) =>
    `${restAPIv1}/searches/${searchId}/completion`,
  chatsMindmap: `${restAPIv1}/chat/mindmap`,
  chatsRelatedQuestions: `${restAPIv1}/chat/recommandation`,

  // next chat
  fetchExternalChatInfo: (id: string) => `${restAPIv1}/chatbots/${id}/info`,

  // file manager
  listFile: `${restAPIv1}/files`,
  uploadFile: `${restAPIv1}/files`,
  removeFile: `${restAPIv1}/files`,
  getAllParentFolder: `${restAPIv1}/files`,
  createFolder: `${restAPIv1}/files`,
  connectFileToKnowledge: `${restAPIv1}/files/link-to-datasets`,
  getFile: `${restAPIv1}/files`,
  moveFile: `${restAPIv1}/files/move`,

  // system
  getSystemVersion: `${restAPIv1}/system/version`,
  getSystemTokenList: `${restAPIv1}/system/tokens`,
  createSystemToken: `${restAPIv1}/system/tokens`,
  removeSystemToken: `${restAPIv1}/system/tokens`,
  getSystemConfig: `${webAPI}/system/config`,
  setLangfuseConfig: `${restAPIv1}/langfuse/api-key`,

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
