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
