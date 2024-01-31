let api_host = `http://223.111.148.200:9380/v1`;

export { api_host };

export default {
  // 用户
  login: `${api_host}/user/login`,
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

  // 上传
  upload: `${api_host}/document/upload`,
  get_document_list: `${api_host}/document/list`,
  document_change_status: `${api_host}/document/change_status`,
  document_rm: `${api_host}/document/rm`,
  document_create: `${api_host}/document/create`,
  document_change_parser: `${api_host}/document/change_parser`,
};
