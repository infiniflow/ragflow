import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const { listFile, removeFile, uploadFile, renameFile } = api;

const methods = {
  listFile: {
    url: listFile,
    method: 'get',
  },
  removeFile: {
    url: removeFile,
    method: 'post',
  },
  uploadFile: {
    url: uploadFile,
    method: 'post',
  },
  renameFile: {
    url: renameFile,
    method: 'post',
  },
} as const;

const chatService = registerServer<keyof typeof methods>(methods, request);

export default chatService;
