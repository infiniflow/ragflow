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
  deleteProviderInstance,
  updateModelStatus,
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
  deleteProviderInstance: {
    url: deleteProviderInstance,
    method: 'delete',
  },
  updateModelStatus: {
    url: updateModelStatus,
    method: 'patch',
  },
} as const;

const llmService = registerNextServer<keyof typeof methods>(methods);

export default llmService;
