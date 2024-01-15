import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  create_kb,
  update_kb,
  rm_kb,
  get_kb_detail,
  kb_list, get_document_list, document_change_status, document_rm, document_create } = api;

const kbService = registerServer(
  {
    createKb: {
      url: create_kb,
      method: 'post'
    },
    updateKb: {
      url: update_kb,
      method: 'post'
    },
    rmKb: {
      url: rm_kb,
      method: 'post'
    },
    get_kb_detail: {
      url: get_kb_detail,
      method: 'get'
    },
    getList: {
      url: kb_list,
      method: 'get'
    },
    get_document_list: {
      url: get_document_list,
      method: 'get'
    },
    document_change_status: {
      url: document_change_status,
      method: 'post'
    },
    document_rm: {
      url: document_rm,
      method: 'post'
    },
    document_create: {
      url: document_create,
      method: 'post'
    },

  },
  request
);

export default kbService;
