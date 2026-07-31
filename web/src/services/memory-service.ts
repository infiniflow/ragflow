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
