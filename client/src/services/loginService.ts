import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  login, } = api;

const chatService = registerServer(
  {
    login: {
      url: login,
      method: 'post'
    }
  },
  request
);

export default chatService;
