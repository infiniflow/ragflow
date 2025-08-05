import message from '@/components/ui/message';
import { ChatSearchParams } from '@/constants/chat';
import { IDialog } from '@/interfaces/database/chat';
import chatService from '@/services/chat-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { history, useSearchParams } from 'umi';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';

export const enum ChatApiAction {
  FetchDialogList = 'fetchDialogList',
  RemoveDialog = 'removeDialog',
  SetDialog = 'setDialog',
}

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
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IDialog[]>({
    queryKey: [
      ChatApiAction.FetchDialogList,
      {
        debouncedSearchString,
        ...pagination,
      },
    ],
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

  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      handleInputChange(e);
    },
    [handleInputChange],
  );

  return {
    data,
    loading,
    refetch,
    searchString,
    handleInputChange: onInputChange,
    pagination: { ...pagination, total: data?.total },
    setPagination,
  };
};

export const useRemoveDialog = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.RemoveDialog],
    mutationFn: async (dialogIds: string[]) => {
      const { data } = await chatService.removeDialog({ dialogIds });
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: ['fetchDialogList'] });

        message.success(t('message.deleted'));
      }
      return data.code;
    },
  });

  return { data, loading, removeDialog: mutateAsync };
};

export const useSetDialog = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.SetDialog],
    mutationFn: async (params: IDialog) => {
      const { data } = await chatService.setDialog(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({
          exact: false,
          queryKey: ['fetchDialogList'],
        });

        queryClient.invalidateQueries({
          queryKey: ['fetchDialog'],
        });
        message.success(
          t(`message.${params.dialog_id ? 'modified' : 'created'}`),
        );
      }
      return data?.code;
    },
  });

  return { data, loading, setDialog: mutateAsync };
};
