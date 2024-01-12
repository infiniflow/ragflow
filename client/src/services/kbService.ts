import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  create_kb,
  update_kb,
  rm_kb,
  update_account,
  account_detail,
  kb_list, } = api;

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
    updateAccount: {
      url: update_account,
      method: 'post'
    },
    getAccountDetail: {
      url: account_detail,
      method: 'post'
    },
    getList: {
      url: kb_list,
      method: 'get'
    }
  },
  request
);

export default kbService;
