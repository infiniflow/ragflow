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
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const { chatChannelSet, chatChannelList } = api;
const methods = {
  chatChannelSet: {
    url: chatChannelSet,
    method: 'post',
  },
  chatChannelList: {
    url: chatChannelList,
    method: 'get',
  },
} as const;

const chatChannelService = registerServer<keyof typeof methods>(
  methods,
  request,
);

export const fetchChatChannelDetail = (id: string) =>
  request.get(api.chatChannelDetail(id));

export const updateChatChannel = (id: string, data: Record<string, any>) =>
  request.patch(api.chatChannelUpdate(id), { data });

export const deleteChatChannel = (id: string) =>
  request.delete(api.chatChannelDel(id));

export const fetchChatChannelRuntime = (id: string) =>
  request.get(api.chatChannelRuntime(id));

export default chatChannelService;
