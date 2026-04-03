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
  createSession,
  listSessions,
  getSession,
  updateSession,
  removeSessions,
  deleteMessage,
  thumbup,
  createToken,
  listToken,
  removeToken,
  getStats,
  chatsTts,
  ask,
  chatsMindmap,
  chatsRelatedQuestions,
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
  createSession: {
    url: createSession,
    method: 'post',
  },
  listSessions: {
    url: listSessions,
    method: 'get',
  },
  getSession: {
    url: getSession,
    method: 'get',
  },
  updateSession: {
    url: updateSession,
    method: 'put',
  },
  removeSessions: {
    url: removeSessions,
    method: 'delete',
  },
  deleteMessage: {
    url: deleteMessage,
    method: 'delete',
  },
  thumbup: {
    url: thumbup,
    method: 'put',
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
  chatsTts: {
    url: chatsTts,
    method: 'post',
  },
  ask: {
    url: ask,
    method: 'post',
  },
  chatsMindmap: {
    url: chatsMindmap,
    method: 'post',
  },
  chatsRelatedQuestions: {
    url: chatsRelatedQuestions,
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
