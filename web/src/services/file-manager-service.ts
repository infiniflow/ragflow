import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  listFile,
  removeFile,
  uploadFile,
  getAllParentFolder,
  createFolder,
  connectFileToKnowledge,
  getDocumentFile,
  getFile,
  moveFile,
  getDocumentFileDownload,
} = api;

const methods = {
  listFile: {
    url: listFile,
    method: 'get',
  },
  removeFile: {
    url: removeFile,
    method: 'delete',
  },
  uploadFile: {
    url: uploadFile,
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
  connectFileToKnowledge: {
    url: connectFileToKnowledge,
    method: 'post',
  },
  getFile: {
    url: getFile,
    method: 'get',
    responseType: 'blob',
  },
  getDocumentFile: {
    url: getDocumentFile,
    method: 'get',
    responseType: 'blob',
  },
  moveFile: {
    url: moveFile,
    method: 'post',
  },
} as const;

const fileManagerService = registerServer<keyof typeof methods>(
  methods,
  request,
);

export const downloadFile = (data: { docId: string; ext: string }) => {
  return request.get(getDocumentFileDownload(data.docId), {
    params: { ext: data.ext },
    responseType: 'blob',
  });
};
export default fileManagerService;
