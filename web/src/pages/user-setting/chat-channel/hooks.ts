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

import message from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import chatChannelService, {
  deleteChatChannel,
  fetchChatChannelDetail,
  updateChatChannel,
} from '@/services/chat-channel-service';
import chatService from '@/services/next-chat-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { t } from 'i18next';
import { useCallback, useMemo, useState } from 'react';
import { fetchAllAgents } from '@/hooks/use-agent-request';
import { ChatChannelKey, useChatChannelInfo } from './constant';
import { IChatChannel, IChatChannelBase, IChatChannelInfo } from './interface';

export const ChatChannelKeys = {
  list: () => ['chat-channel'] as const,
  detail: (id?: string) => ['chat-channel-detail', id] as const,
  dialogs: () => ['chat-channel-dialogs'] as const,
  agents: () => ['chat-channel-agents'] as const,
};

export const useListChatChannel = () => {
  const { chatChannelInfo } = useChatChannelInfo();
  const { data: list, isFetching } = useQuery<IChatChannelBase[]>({
    queryKey: ChatChannelKeys.list(),
    queryFn: async () => {
      const { data } = await chatChannelService.chatChannelList();
      return data.data;
    },
  });

  const categorizedList = useMemo(() => {
    const grouped: Partial<Record<ChatChannelKey, IChatChannelBase[]>> = {};
    (list || []).forEach((item) => {
      const channel = item.channel;
      if (!grouped[channel]) {
        grouped[channel] = [];
      }
      grouped[channel]!.push(item);
    });

    const result: Array<IChatChannelInfo & { list: IChatChannelBase[] }> = [];
    (Object.keys(grouped) as ChatChannelKey[]).forEach((key) => {
      if (chatChannelInfo[key]) {
        result.push({
          id: key,
          name: chatChannelInfo[key].name,
          description: chatChannelInfo[key].description,
          icon: chatChannelInfo[key].icon,
          list: grouped[key] || [],
        });
      }
    });
    return result;
  }, [list, chatChannelInfo]);

  return { list, categorizedList, isFetching };
};

export const useAddChatChannel = () => {
  const [activeChannel, setActiveChannel] = useState<
    IChatChannelInfo | undefined
  >(undefined);
  const [editingRecord, setEditingRecord] = useState<IChatChannel | undefined>(
    undefined,
  );
  const [loading, setLoading] = useState(false);
  const { visible: modalVisible, hideModal, showModal } = useSetModalState();
  const queryClient = useQueryClient();

  const showAddingModal = useCallback(
    (channel: IChatChannelInfo) => {
      setEditingRecord(undefined);
      setActiveChannel(channel);
      showModal();
    },
    [showModal],
  );

  const showEditingModal = useCallback(
    (channel: IChatChannelInfo, record: IChatChannel) => {
      setEditingRecord(record);
      setActiveChannel(channel);
      showModal();
    },
    [showModal],
  );

  const handleOk = useCallback(
    async (values: any) => {
      setLoading(true);
      try {
        const isEdit = Boolean(values?.id);
        const { data: res } = isEdit
          ? await updateChatChannel(values.id, {
              name: values.name,
              config: values.config,
            })
          : await chatChannelService.chatChannelSet(values);
        if (res.code === 0) {
          queryClient.invalidateQueries({ queryKey: ChatChannelKeys.list() });
          if (isEdit) {
            queryClient.invalidateQueries({
              queryKey: ChatChannelKeys.detail(values.id),
            });
          } else if (values.channel === ChatChannelKey.WHATSAPP) {
            setEditingRecord(res.data as IChatChannel);
            queryClient.invalidateQueries({
              queryKey: ChatChannelKeys.detail(res.data?.id),
            });
          }
          message.success(t('message.operated'));
          if (isEdit || values.channel !== ChatChannelKey.WHATSAPP) {
            hideModal();
          }
        }
      } finally {
        setLoading(false);
      }
    },
    [hideModal, queryClient],
  );

  return {
    activeChannel,
    editingRecord,
    loading,
    modalVisible,
    hideModal,
    showAddingModal,
    showEditingModal,
    handleOk,
  };
};

export const useDeleteChatChannel = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation({
    mutationKey: ['delete-chat-channel'],
    mutationFn: async (id: string) => {
      const { data } = await deleteChatChannel(id);
      if (data.code === 0) {
        message.success(t('message.deleted'));
        queryClient.invalidateQueries({ queryKey: ChatChannelKeys.list() });
      }
      return data;
    },
  });
  return { handleDelete: mutateAsync, deleteLoading: isPending };
};

export const useFetchChatChannelDetail = () => {
  const fetchDetail = useCallback(
    async (id: string): Promise<IChatChannel | undefined> => {
      const { data } = await fetchChatChannelDetail(id);
      if (data.code === 0) {
        return data.data;
      }
      return undefined;
    },
    [],
  );
  return { fetchDetail };
};

// Connect (or disconnect) a chat channel to an assistant (dialog) or an agent.
export const useConnectChatChannelTarget = () => {
  const queryClient = useQueryClient();

  const { mutateAsync, isPending } = useMutation({
    mutationKey: ['connect-chat-channel-target'],
    mutationFn: async (params: {
      channelId: string;
      dialogId: string | null;
      agentId: string | null;
    }) => {
      // Only send the target that is actually selected: chat_id and agent_id
      // are mutually exclusive on the server, so a null sibling would
      // otherwise be treated as a disconnection.
      const payload = params.agentId
        ? { agent_id: params.agentId }
        : params.dialogId
          ? { chat_id: params.dialogId }
          : { chat_id: null, agent_id: null };
      const { data } = await updateChatChannel(params.channelId, payload);
      if (data.code === 0) {
        message.success(t('message.operated'));
        queryClient.invalidateQueries({ queryKey: ChatChannelKeys.list() });
      }
      return data;
    },
  });

  return { connect: mutateAsync, connecting: isPending };
};

type ChatChannelTarget = { id: string; name: string };

// Factory for the "assistant / agent" option lists shared by the channel
// connect dialog, so the two hooks stay consistent.
const createChatChannelTargetListHook = (
  queryKey: readonly unknown[],
  fetchTargets: () => Promise<ChatChannelTarget[]>,
) => {
  return () => {
    const { data, isFetching } = useQuery<ChatChannelTarget[]>({
      queryKey,
      initialData: [],
      queryFn: fetchTargets,
    });
    return { targets: data ?? [], isFetching };
  };
};

// Assistants (dialogs) available to connect a channel to.
export const useChatChannelDialogList = createChatChannelTargetListHook(
  ChatChannelKeys.dialogs(),
  async () => {
    const { data } = await chatService.listChats(
      { params: { page_size: 100, page: 1 }, data: {} },
      true,
    );
    return data?.data?.chats ?? [];
  },
);

// Flow agents available to connect a channel to.
export const useChatChannelAgentList = createChatChannelTargetListHook(
  ChatChannelKeys.agents(),
  async () => {
    const agents = await fetchAllAgents();
    return (agents || []).map((agent) => ({
      id: agent.id,
      name: 'title' in agent ? agent.title : agent.id,
    }));
  },
);
