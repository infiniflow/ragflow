import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  getDialog,
  setDialog,
  listDialog,
  removeDialog,
  getConversation,
  setConversation,
  completeConversation,
  listConversation,
  removeConversation,
} = api;

const methods = {
  getDialog: {
    url: getDialog,
    method: 'get',
  },
  setDialog: {
    url: setDialog,
    method: 'post',
  },
  removeDialog: {
    url: removeDialog,
    method: 'post',
  },
  listDialog: {
    url: listDialog,
    method: 'get',
  },
  listConversation: {
    url: listConversation,
    method: 'get',
  },
  getConversation: {
    url: getConversation,
    method: 'get',
  },
  setConversation: {
    url: setConversation,
    method: 'post',
  },
  completeConversation: {
    url: completeConversation,
    method: 'post',
  },
  removeConversation: {
    url: removeConversation,
    method: 'post',
  },
} as const;

const chatService = registerServer<keyof typeof methods>(methods, request);

export default chatService;
