import { ChatSearchParams } from '@/constants/chat';
import { IDialog } from '@/interfaces/database/chat';
import chatService from '@/services/chat-service';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useMemo } from 'react';
import { history, useSearchParams } from 'umi';

export const useGetChatSearchParams = () => {
  const [currentQueryParameters] = useSearchParams();

  return {
    dialogId: currentQueryParameters.get(ChatSearchParams.DialogId) || '',
    conversationId:
      currentQueryParameters.get(ChatSearchParams.ConversationId) || '',
    isNew: currentQueryParameters.get(ChatSearchParams.isNew) || '',
  };
};

export const useClickDialogCard = () => {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const [_, setSearchParams] = useSearchParams();

  const newQueryParameters: URLSearchParams = useMemo(() => {
    return new URLSearchParams();
  }, []);

  const handleClickDialog = useCallback(
    (dialogId: string) => {
      newQueryParameters.set(ChatSearchParams.DialogId, dialogId);
      // newQueryParameters.set(
      //   ChatSearchParams.ConversationId,
      //   EmptyConversationId,
      // );
      setSearchParams(newQueryParameters);
    },
    [newQueryParameters, setSearchParams],
  );

  return { handleClickDialog };
};

export const useFetchDialogList = (pureFetch = false) => {
  const { handleClickDialog } = useClickDialogCard();
  const { dialogId } = useGetChatSearchParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IDialog[]>({
    queryKey: ['fetchDialogList'],
    initialData: [],
    gcTime: 0,
    refetchOnWindowFocus: false,
    queryFn: async (...params) => {
      console.log('🚀 ~ queryFn: ~ params:', params);
      const { data } = await chatService.listDialog();

      if (data.code === 0) {
        const list: IDialog[] = data.data;
        if (!pureFetch) {
          if (list.length > 0) {
            if (list.every((x) => x.id !== dialogId)) {
              handleClickDialog(data.data[0].id);
            }
          } else {
            history.push('/chat');
          }
        }
      }

      return data?.data ?? [];
    },
  });

  return { data, loading, refetch };
};
