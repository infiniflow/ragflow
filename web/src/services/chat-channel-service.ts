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

export default chatChannelService;
