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

import { ChatChannelKey } from './constant';

export interface IChatChannelInfo {
  id: ChatChannelKey;
  name: string;
  description: string;
  icon: React.ReactNode;
}

export interface IChatChannelBase {
  id: string;
  name: string;
  channel: ChatChannelKey;
  // Connected assistant (chat), joined in by the list endpoint.
  chat_id?: string | null;
  dialog_name?: string | null;
}

export type IChatChannel = IChatChannelBase & {
  config: Record<string, any>;
  status: string;
  tenant_id: string;
  create_date?: string;
  update_date?: string;
};

interface IChatChannelInfoItem {
  name: string;
  description: string;
  icon: JSX.Element;
}

export type IChatChannelInfoMap = Record<ChatChannelKey, IChatChannelInfoItem>;
