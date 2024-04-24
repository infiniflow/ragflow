import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  listFile,
  removeFile,
  uploadFile,
  renameFile,
  getAllParentFolder,
  createFolder,
} = api;

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
  getAllParentFolder: {
    url: getAllParentFolder,
    method: 'get',
  },
  createFolder: {
    url: createFolder,
    method: 'post',
  },
} as const;

const fileManagerService = registerServer<keyof typeof methods>(
  methods,
  request,
);

export default fileManagerService;
