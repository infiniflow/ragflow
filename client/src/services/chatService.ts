import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  create_account,
  update_account,
  account_detail,
  getUserDetail, } = api;

const chatService = registerServer(
  {
    createAccount: {
      url: create_account,
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
    getUserDetail: {
      url: getUserDetail,
      method: 'post'
    }
  },
  request
);

export default chatService;
