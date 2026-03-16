import { FileUploadProps } from '@/components/file-upload';
import message from '@/components/ui/message';
import { ChatSearchParams } from '@/constants/chat';
import {
  IClientConversation,
  IConversation,
  IDialog,
  IExternalChatInfo,
} from '@/interfaces/database/chat';
import {
  IAskRequestBody,
  IFeedbackRequestBody,
} from '@/interfaces/request/chat';
import i18n from '@/locales/config';
import { useGetSharedChatSearchParams } from '@/pages/next-chats/hooks/use-send-shared-message';
import { isConversationIdExist } from '@/pages/next-chats/utils';
import chatService from '@/services/next-chat-service';
import api from '@/utils/api';
import { buildMessageListWithUuid, generateConversationId } from '@/utils/chat';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { has } from 'lodash';
import { useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useSearchParams } from 'react-router';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import { useHandleSearchStrChange } from './logic-hooks/use-change-search';

export const enum ChatApiAction {
  FetchDialogList = 'fetchDialogList',
  RemoveDialog = 'removeDialog',
  SetDialog = 'setDialog',
  FetchDialog = 'fetchDialog',
  FetchConversationList = 'fetchConversationList',
  FetchConversation = 'fetchConversation',
  FetchConversationManually = 'fetchConversationManually',
  UpdateConversation = 'updateConversation',
  RemoveConversation = 'removeConversation',
  DeleteMessage = 'deleteMessage',
  FetchMindMap = 'fetchMindMap',
  FetchRelatedQuestions = 'fetchRelatedQuestions',
  UploadAndParse = 'upload_and_parse',
  FetchExternalChatInfo = 'fetchExternalChatInfo',
  Feedback = 'feedback',
  CreateSharedConversation = 'createSharedConversation',
  FetchConversationSse = 'fetchConversationSSE',
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

export const useFetchDialogList = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<{ dialogs: IDialog[]; total: number }>({
    queryKey: [
      ChatApiAction.FetchDialogList,
      {
        debouncedSearchString,
        ...pagination,
      },
    ],
    initialData: { dialogs: [], total: 0 },
    gcTime: 0,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const { data } = await chatService.listDialog(
        {
          params: {
            keywords: debouncedSearchString,
            page_size: pagination.pageSize,
            page: pagination.current,
          },
          data: {},
        },
        true,
      );

      return data?.data ?? { dialogs: [], total: 0 };
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
    mutationFn: async (params: Partial<IDialog>) => {
      const { data } = await chatService.setDialog(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({
          exact: false,
          queryKey: [ChatApiAction.FetchDialogList],
        });

        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchDialog],
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

export const useFetchDialog = () => {
  const { id } = useParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IDialog>({
    queryKey: [ChatApiAction.FetchDialog, id],
    gcTime: 0,
    initialData: {} as IDialog,
    enabled: !!id,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const { data } = await chatService.getDialog(
        { params: { dialogId: id } },
        true,
      );

      return data?.data ?? ({} as IDialog);
    },
  });

  return { data, loading, refetch };
};

//#region Conversation

export const useFetchConversationList = () => {
  const { id } = useParams();

  const { searchString, handleInputChange } = useHandleSearchStrChange();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IConversation[]>({
    queryKey: [ChatApiAction.FetchConversationList, id],
    initialData: [],
    gcTime: 0,
    refetchOnWindowFocus: false,
    enabled: !!id,
    select(data) {
      return searchString
        ? data.filter((x) => x.name.includes(searchString))
        : data;
    },
    queryFn: async () => {
      const { data } = await chatService.listConversation(
        { params: { dialog_id: id } },
        true,
      );
      return data?.data;
    },
  });

  return { data, loading, refetch, searchString, handleInputChange };
};

export function useFetchConversationManually() {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<IClientConversation, unknown, string>({
    mutationKey: [ChatApiAction.FetchConversationManually],
    mutationFn: async (conversationId) => {
      const { data } = await chatService.getConversation(
        {
          params: {
            conversationId,
          },
        },
        true,
      );

      const conversation = data?.data ?? {};

      const messageList = buildMessageListWithUuid(conversation?.message);

      return { ...conversation, message: messageList };
    },
  });

  return { data, loading, fetchConversationManually: mutateAsync };
}

export const useUpdateConversation = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.UpdateConversation],
    mutationFn: async (params: Record<string, any>) => {
      const { data } = await chatService.setConversation({
        ...params,
        conversation_id: params.conversation_id
          ? params.conversation_id
          : generateConversationId(),
      });
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchConversationList],
        });
        message.success(t(`message.modified`));
      }
      return data;
    },
  });

  return { data, loading, updateConversation: mutateAsync };
};

export const useRemoveConversation = () => {
  const queryClient = useQueryClient();
  const { dialogId } = useGetChatSearchParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.RemoveConversation],
    mutationFn: async (conversationIds: string[]) => {
      const { data } = await chatService.removeConversation({
        conversationIds,
        dialogId,
      });
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchConversationList],
        });
      }
      return data.code;
    },
  });

  return { data, loading, removeConversation: mutateAsync };
};

export const useDeleteMessage = () => {
  const { conversationId } = useGetChatSearchParams();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.DeleteMessage],
    mutationFn: async (messageId: string) => {
      const { data } = await chatService.deleteMessage({
        messageId,
        conversationId,
      });

      if (data.code === 0) {
        message.success(t(`message.deleted`));
      }

      return data.code;
    },
  });

  return { data, loading, deleteMessage: mutateAsync };
};

export const useFeedback = () => {
  const { conversationId } = useGetChatSearchParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.Feedback],
    mutationFn: async (params: IFeedbackRequestBody) => {
      const { data } = await chatService.thumbup({
        ...params,
        conversationId,
      });
      if (data.code === 0) {
        message.success(i18n.t(`message.operated`));
      }
      return data.code;
    },
  });

  return { data, loading, feedback: mutateAsync };
};

type UploadParameters = Parameters<NonNullable<FileUploadProps['onUpload']>>;

type X = {
  file: UploadParameters[0][0];
  options: UploadParameters[1];
  conversationId?: string;
};

export function useUploadAndParseFile() {
  const { conversationId: id } = useGetChatSearchParams();
  const { t } = useTranslation();
  const controller = useRef(new AbortController());

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.UploadAndParse],
    mutationFn: async ({
      file,
      options: { onProgress, onSuccess, onError },
      conversationId,
    }: X) => {
      try {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('conversation_id', conversationId || id);

        const { data } = await chatService.uploadAndParse(
          {
            url: api.upload_and_parse,
            signal: controller.current.signal,
            data: formData,
            onUploadProgress: ({ progress }) => {
              onProgress(file, (progress || 0) * 100 - 1);
            },
          },
          true,
        );

        onProgress(file, 100);

        if (data.code === 0) {
          onSuccess(file);
          message.success(t(`message.uploaded`));
        } else {
          onError(file, new Error(data.message));
        }

        return data;
      } catch (error) {
        onError(file, error as Error);
      }
    },
  });

  const cancel = useCallback(() => {
    controller.current.abort();
    controller.current = new AbortController();
  }, [controller]);

  return { data, loading, uploadAndParseFile: mutateAsync, cancel };
}

export const useFetchExternalChatInfo = () => {
  const { sharedId: id } = useGetSharedChatSearchParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IExternalChatInfo>({
    queryKey: [ChatApiAction.FetchExternalChatInfo, id],
    gcTime: 0,
    initialData: {} as IExternalChatInfo,
    enabled: !!id,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const { data } = await chatService.fetchExternalChatInfo(id!);

      return data?.data;
    },
  });

  return { data, loading, refetch };
};

//#endregion

//#region search page

export const useFetchMindMap = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.FetchMindMap],
    gcTime: 0,
    mutationFn: async (params: IAskRequestBody) => {
      try {
        const ret = await chatService.getMindMap(params);
        return ret?.data?.data ?? {};
      } catch (error: any) {
        if (has(error, 'message')) {
          message.error(error.message);
        }

        return [];
      }
    },
  });

  return { data, loading, fetchMindMap: mutateAsync };
};

export const useFetchRelatedQuestions = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.FetchRelatedQuestions],
    gcTime: 0,
    mutationFn: async (question: string): Promise<string[]> => {
      const { data } = await chatService.getRelatedQuestions({ question });

      return data?.data ?? [];
    },
  });

  return { data, loading, fetchRelatedQuestions: mutateAsync };
};
//#endregion

export const useCreateNextSharedConversation = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.CreateSharedConversation],
    mutationFn: async (userId?: string) => {
      const { data } = await chatService.createExternalConversation({ userId });

      return data;
    },
  });

  return { data, loading, createSharedConversation: mutateAsync };
};

export const useFetchNextConversationSSE = () => {
  const { isNew } = useGetChatSearchParams();
  const { sharedId } = useGetSharedChatSearchParams();
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IClientConversation>({
    queryKey: [ChatApiAction.FetchConversationSse, sharedId],
    initialData: {} as IClientConversation,
    gcTime: 0,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      if (isNew !== 'true' && isConversationIdExist(sharedId || '')) {
        if (!sharedId) return {};
        const { data } = await chatService.getConversationSSE(sharedId);
        const conversation = data?.data ?? {};
        const messageList = buildMessageListWithUuid(conversation?.message);
        return { ...conversation, message: messageList };
      }
      return { message: [] };
    },
  });

  return { data, loading, refetch };
};
