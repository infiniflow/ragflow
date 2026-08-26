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
import request from '@/utils/next-request';
import { registerNextServer } from '@/utils/register-server';

const {
  createMemory,
  getMemoryList,
  deleteMemory,
  getMemoryDetail,
  updateMemorySetting,
  getMemoryConfig,
  deleteMemoryMessage,
  getMessageContent,
  updateMessageState,
  // getMemoryDetailShare,
} = api;
const methods = {
  createMemory: {
    url: createMemory,
    method: 'post',
  },
  getMemoryList: {
    url: getMemoryList,
    method: 'get',
  },
  deleteMemory: { url: deleteMemory, method: 'delete' },
  getMemoryConfig: {
    url: getMemoryConfig,
    method: 'get',
  },
  deleteMemoryMessage: { url: deleteMemoryMessage, method: 'delete' },
  getMessageContent: { url: getMessageContent, method: 'get' },
  updateMessageState: { url: updateMessageState, method: 'put' },
} as const;
const memoryService = registerNextServer<keyof typeof methods>(methods);
export const updateMemoryById = (id: string, data: any) => {
  return request.put(updateMemorySetting(id), { ...data });
};
export const getMemoryDetailById = (id: string, data: any) => {
  return request.get(getMemoryDetail(id), { params: data });
};
export default memoryService;
