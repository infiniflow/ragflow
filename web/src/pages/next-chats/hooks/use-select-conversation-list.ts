import { ChatSearchParams, MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import {
  useFetchConversationList,
  useFetchDialogList,
} from '@/hooks/use-chat-request';
import { IConversation } from '@/interfaces/database/chat';
import { getConversationId } from '@/utils/chat';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams, useSearchParams } from 'umi';

export const useFindPrologueFromDialogList = () => {
  const { id: dialogId } = useParams();
  const { data } = useFetchDialogList();

  const prologue = useMemo(() => {
    return data.dialogs.find((x) => x.id === dialogId)?.prompt_config.prologue;
  }, [dialogId, data]);

  return prologue;
};

export const useSetNewConversationRouteParams = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );

  const setNewConversationRouteParams = useCallback(
    (conversationId: string, isNew: string) => {
      newQueryParameters.set(ChatSearchParams.ConversationId, conversationId);
      newQueryParameters.set(ChatSearchParams.isNew, isNew);
      setSearchParams(newQueryParameters);
    },
    [newQueryParameters, setSearchParams],
  );

  return { setNewConversationRouteParams };
};

export const useSelectDerivedConversationList = () => {
  const { t } = useTranslate('chat');

  const [list, setList] = useState<Array<IConversation>>([]);
  const {
    data: conversationList,
    loading,
    handleInputChange,
    searchString,
  } = useFetchConversationList();
  const { id: dialogId } = useParams();
  const { setNewConversationRouteParams } = useSetNewConversationRouteParams();
  const prologue = useFindPrologueFromDialogList();

  const addTemporaryConversation = useCallback(() => {
    const conversationId = getConversationId();
    setList((pre) => {
      if (dialogId) {
        setNewConversationRouteParams(conversationId, 'true');
        const nextList = [
          {
            id: conversationId,
            name: t('newConversation'),
            dialog_id: dialogId,
            is_new: true,
            message: [
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
  }, [conversationList, dialogId, prologue, t, setNewConversationRouteParams]);

  // When you first enter the page, select the top conversation card

  useEffect(() => {
    setList([...conversationList]);
  }, [conversationList]);

  return {
    list,
    addTemporaryConversation,
    loading,
    handleInputChange,
    searchString,
  };
};
