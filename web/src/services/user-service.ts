import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request, { post } from '@/utils/request';

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
  getSystemConfig,
  setLangfuseConfig,
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
  getSystemConfig: {
    url: getSystemConfig,
    method: 'get',
  },
  setLangfuseConfig: {
    url: setLangfuseConfig,
    method: 'put',
  },
  getLangfuseConfig: {
    url: setLangfuseConfig,
    method: 'get',
  },
  deleteLangfuseConfig: {
    url: setLangfuseConfig,
    method: 'delete',
  },
} as const;

const userService = registerServer<keyof typeof methods>(methods, request);

export const listTenantUser = (tenantId: string) =>
  request.get(api.listTenantUser(tenantId));

export const addTenantUser = (tenantId: string, email: string) =>
  post(api.addTenantUser(tenantId), { email });

export const deleteTenantUser = ({
  tenantId,
  userId,
}: {
  tenantId: string;
  userId: string;
}) => request.delete(api.deleteTenantUser(tenantId, userId));

export const listTenant = () => request.get(api.listTenant);

export const agreeTenant = (tenantId: string) =>
  request.put(api.agreeTenant(tenantId));

// 部门相关API
export const listDepartment = (tenantId: string) =>
  request.get(api.listDepartment(tenantId));

export const addDepartment = (tenantId: string, params: { name: string; description?: string; parentId?: string }) =>
  post(api.addDepartment(tenantId), params);

export const updateDepartment = (params: { id: string; name?: string; description?: string; parentId?: string }) =>
  request.put(api.updateDepartment(params.id), params);

export const deleteDepartment = (departmentId: string) =>
  request.delete(api.deleteDepartment(departmentId));

export const addUserToDepartment = (params: { departmentId: string; userIds: string[] }) =>
  post(api.addUserToDepartment(params.departmentId), { userIds: params.userIds });

export const removeUserFromDepartment = (params: { departmentId: string; userId: string }) =>
  request.delete(api.removeUserFromDepartment(params.departmentId, params.userId));

// 群组相关API
export const listGroup = (tenantId: string) =>
  request.get(api.listGroup(tenantId));

export const addGroup = (tenantId: string, params: { name: string; description?: string }) =>
  post(api.addGroup(tenantId), params);

export const updateGroup = (params: { id: string; name?: string; description?: string }) =>
  request.put(api.updateGroup(params.id), params);

export const deleteGroup = (groupId: string) =>
  request.delete(api.deleteGroup(groupId));

export const addUserToGroup = (params: { groupId: string; userIds: string[] }) =>
  post(api.addUserToGroup(params.groupId), { userIds: params.userIds });

export const removeUserFromGroup = (params: { groupId: string; userId: string }) =>
  request.delete(api.removeUserFromGroup(params.groupId, params.userId));

export default userService;
