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

  // llm model
  factories_list: `${api_host}/llm/factories`,
  llm_list: `${api_host}/llm/list`,
  my_llm: `${api_host}/llm/my_llms`,
  set_api_key: `${api_host}/llm/set_api_key`,
  add_llm: `${api_host}/llm/add_llm`,
  delete_llm: `${api_host}/llm/delete_llm`,

  // knowledge base
  kb_list: `${api_host}/kb/list`,
  create_kb: `${api_host}/kb/create`,
  update_kb: `${api_host}/kb/update`,
  rm_kb: `${api_host}/kb/rm`,
  get_kb_detail: `${api_host}/kb/detail`,

  // chunk
  chunk_list: `${api_host}/chunk/list`,
  create_chunk: `${api_host}/chunk/create`,
  set_chunk: `${api_host}/chunk/set`,
  get_chunk: `${api_host}/chunk/get`,
  switch_chunk: `${api_host}/chunk/switch`,
  rm_chunk: `${api_host}/chunk/rm`,
  retrieval_test: `${api_host}/chunk/retrieval_test`,

  // document
  upload: `${api_host}/document/upload`,
  get_document_list: `${api_host}/document/list`,
  document_change_status: `${api_host}/document/change_status`,
  document_rm: `${api_host}/document/rm`,
  document_rename: `${api_host}/document/rename`,
  document_create: `${api_host}/document/create`,
  document_run: `${api_host}/document/run`,
  document_change_parser: `${api_host}/document/change_parser`,
  document_thumbnails: `${api_host}/document/thumbnails`,
  get_document_file: `${api_host}/document/get`,
  document_upload: `${api_host}/document/upload`,
  web_crawl: `${api_host}/document/web_crawl`,

  // chat
  setDialog: `${api_host}/dialog/set`,
  getDialog: `${api_host}/dialog/get`,
  removeDialog: `${api_host}/dialog/rm`,
  listDialog: `${api_host}/dialog/list`,
  setConversation: `${api_host}/conversation/set`,
  getConversation: `${api_host}/conversation/get`,
  listConversation: `${api_host}/conversation/list`,
  removeConversation: `${api_host}/conversation/rm`,
  completeConversation: `${api_host}/conversation/completion`,
  // chat for external
  createToken: `${api_host}/api/new_token`,
  listToken: `${api_host}/api/token_list`,
  removeToken: `${api_host}/api/rm`,
  getStats: `${api_host}/api/stats`,
  createExternalConversation: `${api_host}/api/new_conversation`,
  getExternalConversation: `${api_host}/api/conversation`,
  completeExternalConversation: `${api_host}/api/completion`,

  // file manager
  listFile: `${api_host}/file/list`,
  uploadFile: `${api_host}/file/upload`,
  removeFile: `${api_host}/file/rm`,
  renameFile: `${api_host}/file/rename`,
  getAllParentFolder: `${api_host}/file/all_parent_folder`,
  createFolder: `${api_host}/file/create`,
  connectFileToKnowledge: `${api_host}/file2document/convert`,
  getFile: `${api_host}/file/get`,

  // system
  getSystemVersion: `${api_host}/system/version`,
  getSystemStatus: `${api_host}/system/status`,

  // flow
  listTemplates: `${api_host}/canvas/templates`,
  listCanvas: `${api_host}/canvas/list`,
  getCanvas: `${api_host}/canvas/get`,
  removeCanvas: `${api_host}/canvas/rm`,
  setCanvas: `${api_host}/canvas/set`,
  resetCanvas: `${api_host}/canvas/reset`,
  runCanvas: `${api_host}/canvas/completion`,
};
