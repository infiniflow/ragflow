import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';
import request from '@/utils/request';

const {
  createMemory,
  getMemoryList,
  deleteMemory,
  getMemoryDetail,
  updateMemorySetting,
  getMemoryConfig,
  deleteMemoryMessage,
  getMessageContent,
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
} as const;
const memoryService = registerNextServer<keyof typeof methods>(methods);
export const updateMemoryById = (id: string, data: any) => {
  return request.put(updateMemorySetting(id), { data });
};
export const getMemoryDetailById = (id: string, data: any) => {
  return request.get(getMemoryDetail(id), { params: data });
};
export default memoryService;
