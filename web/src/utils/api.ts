let api_host = `/v1`;

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
  deleteFactory: `${api_host}/llm/delete_factory`,

  // knowledge base
  kb_list: `${api_host}/kb/list`,
  create_kb: `${api_host}/kb/create`,
  update_kb: `${api_host}/kb/update`,
  rm_kb: `${api_host}/kb/rm`,
  get_kb_detail: `${api_host}/kb/detail`,
  getKnowledgeGraph: (knowledgeId: string) =>
    `${api_host}/kb/${knowledgeId}/knowledge_graph`,

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
  document_upload: `${api_host}/document/upload`,
  web_crawl: `${api_host}/document/web_crawl`,
  document_infos: `${api_host}/document/infos`,
  upload_and_parse: `${api_host}/document/upload_and_parse`,
  parse: `${api_host}/document/parse`,
  setMeta: `${api_host}/document/set_meta`,

  // chat
  setDialog: `${api_host}/dialog/set`,
  getDialog: `${api_host}/dialog/get`,
  removeDialog: `${api_host}/dialog/rm`,
  listDialog: `${api_host}/dialog/list`,
  setConversation: `${api_host}/conversation/set`,
  getConversation: `${api_host}/conversation/get`,
  getConversationSSE: `${api_host}/conversation/getsse`,
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

  // flow
  listTemplates: `${api_host}/canvas/templates`,
  listCanvas: `${api_host}/canvas/list`,
  getCanvas: `${api_host}/canvas/get`,
  getCanvasSSE: `${api_host}/canvas/getsse`,
  removeCanvas: `${api_host}/canvas/rm`,
  setCanvas: `${api_host}/canvas/set`,
  resetCanvas: `${api_host}/canvas/reset`,
  runCanvas: `${api_host}/canvas/completion`,
  testDbConnect: `${api_host}/canvas/test_db_connect`,
  getInputElements: `${api_host}/canvas/input_elements`,
  debug: `${api_host}/canvas/debug`,
};
