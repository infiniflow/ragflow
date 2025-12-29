import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';
import request from '@/utils/request';

const {
  createMemory,
  getMemoryList,
  deleteMemory,
  getMemoryDetail,
  updateMemorySetting,
  // getMemoryDetailShare,
} = api;
const methods = {
  createMemory: {
    url: createMemory,
    method: 'post',
  },
  getMemoryList: {
    url: getMemoryList,
    method: 'post',
  },
  deleteMemory: { url: deleteMemory, method: 'post' },
  // getMemoryDetail: {
  //   url: getMemoryDetail,
  //   method: 'get',
  // },
  // updateMemorySetting: {
  //   url: updateMemorySetting,
  //   method: 'post',
  // },
  // getMemoryDetailShare: {
  //   url: getMemoryDetailShare,
  //   method: 'get',
  // },
} as const;
const memoryService = registerNextServer<keyof typeof methods>(methods);
export const updateMemoryById = (id: string, data: any) => {
  return request.post(updateMemorySetting(id), { data });
};
export const getMemoryDetailById = (id: string, data: any) => {
  return request.post(getMemoryDetail(id), { data });
};
export default memoryService;
