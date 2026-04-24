const webAPI = `/v1`;
const restAPIv1 = `/api/v1`;

export { restAPIv1, webAPI };

export default {
  // user
  login: `${restAPIv1}/auth/login`,
  logout: `${restAPIv1}/auth/logout`,
  register: `${restAPIv1}/users`,
  setting: `${restAPIv1}/users/me`,
  userInfo: `${restAPIv1}/users/me`,
  tenantInfo: `${restAPIv1}/users/me/models`,
  setTenantInfo: `${restAPIv1}/users/me/models`,
  loginChannels: `${restAPIv1}/auth/login/channels`,
  loginChannel: (channel: string) => `${restAPIv1}/auth/login/${channel}`,

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
  llmTools: `${restAPIv1}/plugin/tools`,

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
  pipelineRerun: `${restAPIv1}/agents/rerun`,
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
  chunkList: (datasetId: string, documentId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents/${documentId}/chunks`,
  chunkDetail: (datasetId: string, documentId: string, chunkId: string) =>
    `${restAPIv1}/datasets/${datasetId}/documents/${documentId}/chunks/${chunkId}`,
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
  getSystemConfig: `${restAPIv1}/system/config`,
  setLangfuseConfig: `${restAPIv1}/langfuse/api-key`,

  // flow
  listAgentTemplate: `${restAPIv1}/agents/templates`,
  listAgents: `${restAPIv1}/agents`,
  createAgent: `${restAPIv1}/agents`,
  updateAgent: (agentId: string) => `${restAPIv1}/agents/${agentId}`,
  deleteAgent: (agentId: string) => `${restAPIv1}/agents/${agentId}`,
  agentChatCompletion: `${restAPIv1}/agents/chat/completion`,
  resetAgent: (agentId: string) => `${restAPIv1}/agents/${agentId}/reset`,
  testDbConnect: `${restAPIv1}/agents/test_db_connection`,
  getInputElements: `${webAPI}/canvas/input_elements`,
  debug: (agentId: string, componentId: string) =>
    `${restAPIv1}/agents/${agentId}/components/${componentId}/debug`,
  trace: (agentId: string, messageId: string) =>
    `${restAPIv1}/agents/${agentId}/logs/${messageId}`,
  cancelCanvas: (taskId: string) => `${webAPI}/canvas/cancel/${taskId}`, // cancel conversation
  // agent
  inputForm: (agentId: string, componentId: string) =>
    `${restAPIv1}/agents/${agentId}/components/${componentId}/input-form`,
  fetchVersionList: (id: string) => `${restAPIv1}/agents/${id}/versions`,
  fetchVersion: (agentId: string, versionId: string) =>
    `${restAPIv1}/agents/${agentId}/versions/${versionId}`,
  getAgent: (id: string) => `${restAPIv1}/agents/${id}`,
  uploadAgentFile: (id?: string) => `${restAPIv1}/agents/${id}/upload`,
  createAgentSession: (agentId: string) =>
    `${restAPIv1}/agents/${agentId}/sessions`,
  fetchAgentLogs: (canvasId: string) => `${webAPI}/canvas/${canvasId}/sessions`,
  fetchAgentSessions: (agentId: string) =>
    `${restAPIv1}/agents/${agentId}/sessions`,
  fetchAgentSessionById: (agentId: string, sessionId: string) =>
    `${restAPIv1}/agents/${agentId}/sessions/${sessionId}`,
  fetchExternalAgentInputs: (canvasId: string) =>
    `${restAPIv1}/agentbots/${canvasId}/inputs`,
  prompt: `${restAPIv1}/agents/prompts`,
  cancelDataflow: (id: string) => `${webAPI}/canvas/cancel/${id}`,
  downloadFile: `${restAPIv1}/agents/download`,
  testWebhook: (id: string) => `${restAPIv1}/webhook_test/${id}`,
  fetchWebhookTrace: (id: string) => `${restAPIv1}/webhook_trace/${id}`,

  // explore

  // mcp server
  listMcpServer: `${restAPIv1}/mcp/servers`,
  getMcpServer: (id: string) => `${restAPIv1}/mcp/servers/${id}`,
  createMcpServer: `${restAPIv1}/mcp/servers`,
  updateMcpServer: (id: string) => `${restAPIv1}/mcp/servers/${id}`,
  deleteMcpServer: (id: string) => `${restAPIv1}/mcp/servers/${id}`,
  importMcpServer: `${restAPIv1}/mcp/servers/import`,
  exportMcpServer: (id: string) =>
    `${restAPIv1}/mcp/servers/${id}?mode=download`,
  testMcpServer: (id: string) => `${restAPIv1}/mcp/servers/${id}/test`,

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

  // Skill spaces
  skillSpaces: `${restAPIv1}/skills/spaces`,
  skillSpace: (spaceId: string) => `${restAPIv1}/skills/spaces/${spaceId}`,
  skillSpaceByFolder: `${restAPIv1}/skills/space/by-folder`,
  skillConfig: `${restAPIv1}/skills/config`,
  skillSearch: `${restAPIv1}/skills/search`,
  skillIndex: `${restAPIv1}/skills/index`,
  skillReindex: `${restAPIv1}/skills/reindex`,
};
