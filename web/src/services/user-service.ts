import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request, { post } from '@/utils/request';

const {
  login,
  logout,
  register,
  setting,
  userInfo,
  tenantInfo,
  factoriesList,
  llmList,
  myLlm,
  setApiKey,
  setTenantInfo,
  addLlm,
  deleteLlm,
  enableLlm,
  deleteFactory,
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
    method: 'post',
  },
  register: {
    url: register,
    method: 'post',
  },
  setting: {
    url: setting,
    method: 'patch',
  },
  userInfo: {
    url: userInfo,
    method: 'get',
  },
  getTenantInfo: {
    url: tenantInfo,
    method: 'get',
  },
  setTenantInfo: {
    url: setTenantInfo,
    method: 'patch',
  },
  factoriesList: {
    url: factoriesList,
    method: 'get',
  },
  llmList: {
    url: llmList,
    method: 'get',
  },
  myLlm: {
    url: myLlm,
    method: 'get',
  },
  setApiKey: {
    url: setApiKey,
    method: 'post',
  },
  addLlm: {
    url: addLlm,
    method: 'post',
  },
  deleteLlm: {
    url: deleteLlm,
    method: 'post',
  },
  enableLlm: {
    url: enableLlm,
    method: 'post',
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

export const getLoginChannels = () => request.get(api.loginChannels);
export const loginWithChannel = (channel: string) =>
  (window.location.href = api.loginChannel(channel));

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
}) =>
  request.delete(api.deleteTenantUser(tenantId), {
    data: { userId },
  });

export const listTenant = () => request.get(api.listTenant);

export const agreeTenant = (tenantId: string) =>
  request.patch(api.agreeTenant(tenantId));

export default userService;
