/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
  chatsTts,
  chatsMindmap,
  chatsRelatedQuestions,
  documentInfoUpload,
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
    method: 'patch',
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
  chatsTts: {
    url: chatsTts,
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
  documentInfoUpload: {
    method: 'post',
    url: documentInfoUpload,
  },
  fetchExternalChatInfo: {
    url: fetchExternalChatInfo,
    method: 'get',
  },
} as const;

const chatService = registerNextServer<keyof typeof methods>(methods);

export default chatService;
