import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import {
  useFetchChatList,
  useFetchSessionList,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { IConversation } from '@/interfaces/database/chat';
import { generateConversationId } from '@/utils/chat';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router';
import { useChatUrlParams } from './use-chat-url';

export const useFindPrologueFromDialogList = () => {
  const { id: dialogId } = useParams();
  const { data } = useFetchChatList();

  const prologue = useMemo(() => {
    return data?.chats.find((x) => x.id === dialogId)?.prompt_config?.prologue;
  }, [dialogId, data]);

  return prologue;
};

export const useSelectDerivedConversationList = () => {
  const { t } = useTranslate('chat');

  const [list, setList] = useState<Array<IConversation>>([]);
  const {
    data: conversationList,
    total,
    loading,
    loadingMore,
    handleScroll,
    handleInputChange,
    searchString,
    setSearchString,
  } = useFetchSessionList();

  const { id: dialogId } = useParams();
  const prologue = useFindPrologueFromDialogList();
  const { setConversationBoth } = useChatUrlParams();
  const { isNew } = useGetChatSearchParams();

  const addTemporaryConversation = useCallback(() => {
    const conversationId = generateConversationId();
    // Clear the search keyword, otherwise the newly created session will be
    // filtered out by the search after it is persisted and refetched.
    setSearchString('');
    setList((pre) => {
      if (dialogId) {
        setConversationBoth(conversationId, 'true');
        const nextList = [
          {
            id: conversationId,
            name: t('newConversation'),
            chat_id: dialogId,
            is_new: true,
            messages: [
              {
                content: prologue,
                role: MessageType.Assistant,
              },
            ],
          } as any,
          ...conversationList,
        ];
        return nextList;
      }

      return pre;
    });
  }, [
    dialogId,
    setConversationBoth,
    t,
    prologue,
    conversationList,
    setSearchString,
  ]);

  const removeTemporaryConversation = useCallback((conversationId: string) => {
    setList((prevList) => {
      return prevList.filter(
        (conversation) => conversation.id !== conversationId,
      );
    });
  }, []);

  useEffect(() => {
    setList((previous) => [
      ...previous.filter(
        (session) =>
          session.is_new && session.chat_id === dialogId && isNew === 'true',
      ),
      ...conversationList,
    ]);
  }, [conversationList, dialogId, isNew]);

  return {
    list,
    total,
    addTemporaryConversation,
    removeTemporaryConversation,
    loading,
    loadingMore,
    handleScroll,
    handleInputChange,
    searchString,
  };
};
