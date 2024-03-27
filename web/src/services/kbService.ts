import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';
import pureRequest from 'umi-request';

const {
  create_kb,
  update_kb,
  rm_kb,
  get_kb_detail,
  kb_list,
  get_document_list,
  document_change_status,
  document_rm,
  document_create,
  document_change_parser,
  document_thumbnails,
  chunk_list,
  create_chunk,
  set_chunk,
  get_chunk,
  switch_chunk,
  rm_chunk,
  retrieval_test,
  document_rename,
  document_run,
  get_document_file,
  document_upload,
} = api;

const methods = {
  // 知识库管理
  createKb: {
    url: create_kb,
    method: 'post',
  },
  updateKb: {
    url: update_kb,
    method: 'post',
  },
  rmKb: {
    url: rm_kb,
    method: 'post',
  },
  get_kb_detail: {
    url: get_kb_detail,
    method: 'get',
  },
  getList: {
    url: kb_list,
    method: 'get',
  },
  // 文件管理
  get_document_list: {
    url: get_document_list,
    method: 'get',
  },
  document_change_status: {
    url: document_change_status,
    method: 'post',
  },
  document_rm: {
    url: document_rm,
    method: 'post',
  },
  document_rename: {
    url: document_rename,
    method: 'post',
  },
  document_create: {
    url: document_create,
    method: 'post',
  },
  document_run: {
    url: document_run,
    method: 'post',
  },
  document_change_parser: {
    url: document_change_parser,
    method: 'post',
  },
  document_thumbnails: {
    url: document_thumbnails,
    method: 'get',
  },
  document_upload: {
    url: document_upload,
    method: 'post',
  },
  // chunk管理
  chunk_list: {
    url: chunk_list,
    method: 'post',
  },
  create_chunk: {
    url: create_chunk,
    method: 'post',
  },
  set_chunk: {
    url: set_chunk,
    method: 'post',
  },
  get_chunk: {
    url: get_chunk,
    method: 'get',
  },
  switch_chunk: {
    url: switch_chunk,
    method: 'post',
  },
  rm_chunk: {
    url: rm_chunk,
    method: 'post',
  },
  retrieval_test: {
    url: retrieval_test,
    method: 'post',
  },
};

const kbService = registerServer<keyof typeof methods>(methods, request);

export const getDocumentFile = (documentId: string) => {
  return pureRequest(get_document_file + '/' + documentId, {
    responseType: 'blob',
    method: 'get',
    parseResponse: false,
    // getResponse: true,
  })
    .then((res) => {
      const x = res.headers.get('content-disposition');
      console.info(res);
      console.info(x);
      return res.blob();
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

export default kbService;
