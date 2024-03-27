let api_host = `/v1`;

export { api_host };

export default {
  // 用户
  login: `${api_host}/user/login`,
  logout: `${api_host}/user/logout`,
  register: `${api_host}/user/register`,
  setting: `${api_host}/user/setting`,
  user_info: `${api_host}/user/info`,
  tenant_info: `${api_host}/user/tenant_info`,
  set_tenant_info: `${api_host}/user/set_tenant_info`,

  // 模型管理
  factories_list: `${api_host}/llm/factories`,
  llm_list: `${api_host}/llm/list`,
  my_llm: `${api_host}/llm/my_llms`,
  set_api_key: `${api_host}/llm/set_api_key`,

  //知识库管理
  kb_list: `${api_host}/kb/list`,
  create_kb: `${api_host}/kb/create`,
  update_kb: `${api_host}/kb/update`,
  rm_kb: `${api_host}/kb/rm`,
  get_kb_detail: `${api_host}/kb/detail`,

  // chunk管理
  chunk_list: `${api_host}/chunk/list`,
  create_chunk: `${api_host}/chunk/create`,
  set_chunk: `${api_host}/chunk/set`,
  get_chunk: `${api_host}/chunk/get`,
  switch_chunk: `${api_host}/chunk/switch`,
  rm_chunk: `${api_host}/chunk/rm`,
  retrieval_test: `${api_host}/chunk/retrieval_test`,

  // 文件管理
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

  setDialog: `${api_host}/dialog/set`,
  getDialog: `${api_host}/dialog/get`,
  removeDialog: `${api_host}/dialog/rm`,
  listDialog: `${api_host}/dialog/list`,

  setConversation: `${api_host}/conversation/set`,
  getConversation: `${api_host}/conversation/get`,
  listConversation: `${api_host}/conversation/list`,
  removeConversation: `${api_host}/conversation/rm`,
  completeConversation: `${api_host}/conversation/completion`,
};
