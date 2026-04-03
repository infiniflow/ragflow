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
import chatService from '@/services/next-chat-service';
import api from '@/utils/api';
import { buildMessageListWithUuid } from '@/utils/chat';
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
  FetchChatList = 'fetchChatList',
  DeleteChat = 'deleteChat',
  CreateChat = 'createChat',
  UpdateChat = 'updateChat',
  PatchChat = 'patchChat',
  FetchChat = 'fetchChat',
  FetchSessionList = 'fetchSessionList',
  FetchSession = 'fetchSession',
  FetchSessionManually = 'fetchSessionManually',
  CreateSession = 'createSession',
  UpdateSession = 'updateSession',
  RemoveSession = 'removeSession',
  DeleteMessage = 'deleteMessage',
  FetchMindMap = 'fetchMindMap',
  FetchRelatedQuestions = 'fetchRelatedQuestions',
  UploadAndParse = 'upload_and_parse',
  FetchExternalChatInfo = 'fetchExternalChatInfo',
  Feedback = 'feedback',
  CreateSharedConversation = 'createSharedConversation',
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

export const useFetchChatList = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<{ chats: IDialog[]; total: number }>({
    queryKey: [
      ChatApiAction.FetchChatList,
      {
        debouncedSearchString,
        ...pagination,
      },
    ],
    initialData: { chats: [], total: 0 },
    gcTime: 0,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const { data } = await chatService.listChats(
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

      return data?.data ?? { chats: [], total: 0 };
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

export const useDeleteChat = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.DeleteChat],
    mutationFn: async (chatId: string) => {
      const { data } = await chatService.deleteChat(chatId);
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchChatList],
        });
        message.success(t('message.deleted'));
      }
      return data.code;
    },
  });

  return { data, loading, deleteChat: mutateAsync };
};

export const useCreateChat = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.CreateChat],
    mutationFn: async (params: Record<string, any>) => {
      const { data } = await chatService.createChat(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({
          exact: false,
          queryKey: [ChatApiAction.FetchChatList],
        });
        message.success(t('message.created'));
      }
      return data?.code;
    },
  });

  return { data, loading, createChat: mutateAsync };
};

export const useUpdateChat = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.UpdateChat],
    mutationFn: async ({
      chatId,
      params,
    }: {
      chatId: string;
      params: Record<string, any>;
    }) => {
      const { data } = await chatService.updateChat(
        { url: api.updateChat(chatId), data: params },
        true,
      );
      if (data.code === 0) {
        queryClient.invalidateQueries({
          exact: false,
          queryKey: [ChatApiAction.FetchChatList],
        });
        queryClient.invalidateQueries({ queryKey: [ChatApiAction.FetchChat] });
        message.success(t('message.modified'));
      }
      return data?.code;
    },
  });

  return { data, loading, updateChat: mutateAsync };
};

export const usePatchChat = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.PatchChat],
    mutationFn: async ({
      chatId,
      params,
    }: {
      chatId: string;
      params: Record<string, any>;
    }) => {
      const { data } = await chatService.patchChat(
        { url: api.patchChat(chatId), data: params },
        true,
      );
      if (data.code === 0) {
        queryClient.invalidateQueries({
          exact: false,
          queryKey: [ChatApiAction.FetchChatList],
        });
        queryClient.invalidateQueries({ queryKey: [ChatApiAction.FetchChat] });
        message.success(t('message.modified'));
      }
      return data?.code;
    },
  });

  return { data, loading, patchChat: mutateAsync };
};

export const useFetchChat = () => {
  const { id } = useParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IDialog>({
    queryKey: [ChatApiAction.FetchChat, id],
    gcTime: 0,
    initialData: {} as IDialog,
    enabled: !!id,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const { data } = await chatService.getChat(id);
      return data?.data ?? ({} as IDialog);
    },
  });

  return { data, loading, refetch };
};

//#region Session

export const useFetchSessionList = () => {
  const { id } = useParams();

  const { searchString, handleInputChange } = useHandleSearchStrChange();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IConversation[]>({
    queryKey: [ChatApiAction.FetchSessionList, id],
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
      const { data } = await chatService.listSessions(
        { url: api.listSessions(id!) },
        true,
      );
      return data?.data;
    },
  });

  return { data, loading, refetch, searchString, handleInputChange };
};

export function useFetchSessionManually() {
  const { id: chatId } = useParams();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<IClientConversation, unknown, string>({
    mutationKey: [ChatApiAction.FetchSessionManually],
    mutationFn: async (sessionId) => {
      const { data } = await chatService.getSession(
        { url: api.getSession(chatId!, sessionId) },
        true,
      );

      const conversation = data?.data ?? {};

      const messageList = buildMessageListWithUuid(conversation?.messages);

      return { ...conversation, messages: messageList };
    },
  });

  return { data, loading, fetchSessionManually: mutateAsync };
}

export const useCreateSession = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.CreateSession],
    mutationFn: async ({ chatId, name }: { chatId: string; name: string }) => {
      const { data } = await chatService.createSession(
        { url: api.createSession(chatId), data: { name } },
        true,
      );
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchSessionList],
        });
      }
      return data;
    },
  });

  return { data, loading, createSession: mutateAsync };
};

export const useUpdateSession = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.UpdateSession],
    mutationFn: async ({
      chatId,
      sessionId,
      params,
    }: {
      chatId: string;
      sessionId: string;
      params: Record<string, any>;
    }) => {
      const { data } = await chatService.updateSession(
        { url: api.updateSession(chatId, sessionId), data: params },
        true,
      );
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchSessionList],
        });
        message.success(t(`message.modified`));
      }
      return data;
    },
  });

  return { data, loading, updateSession: mutateAsync };
};

export const useRemoveSessions = () => {
  const queryClient = useQueryClient();
  const { id: chatId } = useParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.RemoveSession],
    mutationFn: async (sessionIds: string[]) => {
      const { data } = await chatService.removeSessions(
        { url: api.removeSessions(chatId!), data: { ids: sessionIds } },
        true,
      );
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [ChatApiAction.FetchSessionList],
        });
      }
      return data.code;
    },
  });

  return { data, loading, removeSessions: mutateAsync };
};

export const useDeleteMessage = () => {
  const { conversationId } = useGetChatSearchParams();
  const { id: chatId } = useParams();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.DeleteMessage],
    mutationFn: async (messageId: string) => {
      const { data } = await chatService.deleteMessage(
        { url: api.deleteMessage(chatId!, conversationId, messageId) },
        true,
      );

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
  const { id: chatId } = useParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [ChatApiAction.Feedback],
    mutationFn: async (params: IFeedbackRequestBody) => {
      const { data } = await chatService.thumbup(
        {
          url: api.thumbup(chatId!, conversationId, params.messageId!),
          data: { thumbup: params.thumbup, feedback: params.feedback },
        },
        true,
      );
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

//#endregion Session

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
        const ret = await chatService.chatsMindmap(params);
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
      const { data } = await chatService.chatsRelatedQuestions({ question });

      return data?.data ?? [];
    },
  });

  return { data, loading, fetchRelatedQuestions: mutateAsync };
};
//#endregion
