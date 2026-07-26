import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

const {
  listAllAddedModels,
  defaultModel,
  listProviders,
  addProvider,
  addProviderInstance,
  verifyProviderConnection,
  listProviderModels,
  listProviderInstances,
  listInstanceModels,
  showProviderInstance,
  addInstanceModel,
  editInstanceModel,
  deleteProviderInstance,
  updateModelStatus,
  patchInstanceModel,
  deleteInstanceModels,
  updateProviderInstance,
  aimlapiAuthorizeStart,
  aimlapiAuthorizePoll,
} = api;

const methods = {
  listAllAddedModels: {
    url: listAllAddedModels,
    method: 'get',
  },
  listDefaultModels: {
    url: defaultModel,
    method: 'get',
  },
  setDefaultModel: {
    url: defaultModel,
    method: 'patch',
  },
  listProviders: {
    url: listProviders,
    method: 'get',
  },
  addProvider: {
    url: addProvider,
    method: 'put',
  },
  addProviderInstance: {
    url: addProviderInstance,
    method: 'post',
  },
  verifyProviderConnection: {
    url: verifyProviderConnection,
    method: 'post',
  },
  listProviderModels: {
    url: listProviderModels,
    method: 'get',
  },
  listProviderInstances: {
    url: listProviderInstances,
    method: 'get',
  },
  listInstanceModels: {
    url: listInstanceModels,
    method: 'get',
  },
  showProviderInstance: {
    url: showProviderInstance,
    method: 'get',
  },
  addInstanceModel: {
    url: addInstanceModel,
    method: 'post',
  },
  editInstanceModel: {
    url: editInstanceModel,
    method: 'put',
  },
  deleteProviderInstance: {
    url: deleteProviderInstance,
    method: 'delete',
  },
  updateModelStatus: {
    url: updateModelStatus,
    method: 'patch',
  },
  patchInstanceModel: {
    url: patchInstanceModel,
    method: 'patch',
  },
  deleteInstanceModels: {
    url: deleteInstanceModels,
    method: 'delete',
  },
  updateProviderInstance: {
    url: updateProviderInstance,
    method: 'put',
  },
  aimlapiAuthorizeStart: {
    url: aimlapiAuthorizeStart,
    method: 'post',
  },
  aimlapiAuthorizePoll: {
    url: aimlapiAuthorizePoll,
    method: 'post',
  },
} as const;

const llmService = registerNextServer<keyof typeof methods>(methods);

export default llmService;
