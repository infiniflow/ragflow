import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  getDialog,
  setDialog,
  listDialog,
  removeDialog,
  getConversation,
  getConversationSSE,
  setConversation,
  completeConversation,
  listConversation,
  removeConversation,
  createToken,
  listToken,
  removeToken,
  getStats,
  createExternalConversation,
  getExternalConversation,
  completeExternalConversation,
  uploadAndParseExternal,
  deleteMessage,
  thumbup,
  tts,
  ask,
  mindmap,
  getRelatedQuestions,
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
  getConversationSSE: {
    url: getConversationSSE,
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
  createToken: {
    url: createToken,
    method: 'post',
  },
  listToken: {
    url: listToken,
    method: 'get',
  },
  removeToken: {
    url: removeToken,
    method: 'post',
  },
  getStats: {
    url: getStats,
    method: 'get',
  },
  createExternalConversation: {
    url: createExternalConversation,
    method: 'get',
  },
  getExternalConversation: {
    url: getExternalConversation,
    method: 'get',
  },
  completeExternalConversation: {
    url: completeExternalConversation,
    method: 'post',
  },
  uploadAndParseExternal: {
    url: uploadAndParseExternal,
    method: 'post',
  },
  deleteMessage: {
    url: deleteMessage,
    method: 'post',
  },
  thumbup: {
    url: thumbup,
    method: 'post',
  },
  tts: {
    url: tts,
    method: 'post',
  },
  ask: {
    url: ask,
    method: 'post',
  },
  getMindMap: {
    url: mindmap,
    method: 'post',
  },
  getRelatedQuestions: {
    url: getRelatedQuestions,
    method: 'post',
  },
} as const;

const chatService = registerServer<keyof typeof methods>(methods, request);

export default chatService;
