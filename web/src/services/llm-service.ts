/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
