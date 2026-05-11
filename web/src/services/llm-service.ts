import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

const {
  listProviders,
  addProvider,
  addProviderInstance,
  listProviderInstances,
  listInstanceModels,
  deleteProviderInstance,
  updateModelStatus,
} = api;

const methods = {
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
  listProviderInstances: {
    url: listProviderInstances,
    method: 'get',
  },
  listInstanceModels: {
    url: listInstanceModels,
    method: 'get',
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
