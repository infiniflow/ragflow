import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';
import pureRequest from 'axios';

const {
  listFile,
  removeFile,
  uploadFile,
  renameFile,
  getAllParentFolder,
  createFolder,
  connectFileToKnowledge,
  get_document_file,
  getFile,
  moveFile,
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
    url: get_document_file,
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

export default fileManagerService;

export const getDocumentFile = (documentId: string) => {
  return pureRequest(getFile + '/' + documentId, {
    responseType: 'blob',
    method: 'get',
    // headers: {
    //   'content-type':
    //     'text/plain;charset=UTF-8, application/vnd.openxmlformats-officeddocument.spreadsheetml.sheet;charset=UTF-8',
    // },

    // parseResponse: false,
    // getResponse: true,
  })
    .then((res) => {
      const x = res?.headers?.get('content-disposition');
      const y = res?.headers?.get('Content-Type');
      console.info(res);
      console.info(x);
      console.info('Content-Type', y);
      return res;
    })
    .then((res) => {
      // const objectURL = URL.createObjectURL(res);

      // let btn = document.createElement('a');

      // btn.download = '文件名.pdf';

      // btn.href = objectURL;

      // btn.click();

      // URL.revokeObjectURL(objectURL);

      // btn = null;

      return res;
    })
    .catch((err) => {
      console.info(err);
    });
};
