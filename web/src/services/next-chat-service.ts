import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

const {
  createChat,
  listChats,
  getChat,
  updateChat,
  patchChat,
  deleteChat,
  bulkDeleteChats,
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
  upload_and_parse,
  fetchExternalChatInfo,
} = api;

const methods = {
  createChat: {
    url: createChat,
    method: 'post',
  },
  listChats: {
    url: listChats,
    method: 'get',
  },
  getChat: {
    url: getChat,
    method: 'get',
  },
  updateChat: {
    url: updateChat,
    method: 'put',
  },
  patchChat: {
    url: patchChat,
    method: 'patch',
  },
  deleteChat: {
    url: deleteChat,
    method: 'delete',
  },
  bulkDeleteChats: {
    url: bulkDeleteChats,
    method: 'delete',
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
  uploadAndParse: {
    method: 'post',
    url: upload_and_parse,
  },
  fetchExternalChatInfo: {
    url: fetchExternalChatInfo,
    method: 'get',
  },
} as const;

const chatService = registerNextServer<keyof typeof methods>(methods);

export default chatService;
