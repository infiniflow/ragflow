import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  login,
  logout,
  register,
  setting,
  user_info,
  tenant_info,
  factories_list,
  llm_list,
  my_llm,
  set_api_key,
  set_tenant_info,
  add_llm,
  delete_llm,
  deleteFactory,
  getSystemStatus,
  getSystemVersion,
  getSystemTokenList,
  removeSystemToken,
  createSystemToken,
} = api;

const methods = {
  login: {
    url: login,
    method: 'post',
  },
  logout: {
    url: logout,
    method: 'get',
  },
  register: {
    url: register,
    method: 'post',
  },
  setting: {
    url: setting,
    method: 'post',
  },
  user_info: {
    url: user_info,
    method: 'get',
  },
  get_tenant_info: {
    url: tenant_info,
    method: 'get',
  },
  set_tenant_info: {
    url: set_tenant_info,
    method: 'post',
  },
  factories_list: {
    url: factories_list,
    method: 'get',
  },
  llm_list: {
    url: llm_list,
    method: 'get',
  },
  my_llm: {
    url: my_llm,
    method: 'get',
  },
  set_api_key: {
    url: set_api_key,
    method: 'post',
  },
  add_llm: {
    url: add_llm,
    method: 'post',
  },
  delete_llm: {
    url: delete_llm,
    method: 'post',
  },
  getSystemStatus: {
    url: getSystemStatus,
    method: 'get',
  },
  getSystemVersion: {
    url: getSystemVersion,
    method: 'get',
  },
  deleteFactory: {
    url: deleteFactory,
    method: 'post',
  },
  listToken: {
    url: getSystemTokenList,
    method: 'get',
  },
  createToken: {
    url: createSystemToken,
    method: 'post',
  },
  removeToken: {
    url: removeSystemToken,
    method: 'delete',
  },
} as const;

const userService = registerServer<keyof typeof methods>(methods, request);

export default userService;
