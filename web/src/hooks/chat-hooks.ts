import {
  IConversation,
  IDialog,
  IStats,
  IToken,
} from '@/interfaces/database/chat';
import chatService from '@/services/chat-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import dayjs, { Dayjs } from 'dayjs';
import { useCallback, useState } from 'react';
import { useDispatch, useSelector } from 'umi';

export const useFetchDialogList = () => {
  const dispatch = useDispatch();

  const fetchDialogList = useCallback(() => {
    return dispatch<any>({ type: 'chatModel/listDialog' });
  }, [dispatch]);

  return fetchDialogList;
};

export const useSelectDialogList = () => {
  const dialogList: IDialog[] = useSelector(
    (state: any) => state.chatModel.dialogList,
  );

  return dialogList;
};

export const useFetchConversationList = () => {
  const dispatch = useDispatch();

  const fetchConversationList = useCallback(
    async (dialogId: string) => {
      if (dialogId) {
        dispatch({
          type: 'chatModel/listConversation',
          payload: { dialog_id: dialogId },
        });
      }
    },
    [dispatch],
  );

  return fetchConversationList;
};

export const useSelectConversationList = () => {
  const conversationList: IConversation[] = useSelector(
    (state: any) => state.chatModel.conversationList,
  );

  return conversationList;
};

export const useFetchConversation = () => {
  const dispatch = useDispatch();

  const fetchConversation = useCallback(
    (conversationId: string, needToBeSaved = true) => {
      return dispatch<any>({
        type: 'chatModel/getConversation',
        payload: {
          needToBeSaved,
          conversation_id: conversationId,
        },
      });
    },
    [dispatch],
  );

  return fetchConversation;
};

export const useFetchDialog = () => {
  const dispatch = useDispatch();

  const fetchDialog = useCallback(
    (dialogId: string, needToBeSaved = true) => {
      if (dialogId) {
        return dispatch<any>({
          type: 'chatModel/getDialog',
          payload: { dialog_id: dialogId, needToBeSaved },
        });
      }
    },
    [dispatch],
  );

  return fetchDialog;
};

export const useRemoveDialog = () => {
  const dispatch = useDispatch();

  const removeDocument = useCallback(
    (dialogIds: Array<string>) => {
      return dispatch({
        type: 'chatModel/removeDialog',
        payload: {
          dialog_ids: dialogIds,
        },
      });
    },
    [dispatch],
  );

  return removeDocument;
};

export const useUpdateConversation = () => {
  const dispatch = useDispatch();

  const updateConversation = useCallback(
    (payload: any) => {
      return dispatch<any>({
        type: 'chatModel/setConversation',
        payload,
      });
    },
    [dispatch],
  );

  return updateConversation;
};

export const useSetDialog = () => {
  const dispatch = useDispatch();

  const setDialog = useCallback(
    (payload: IDialog) => {
      return dispatch<any>({ type: 'chatModel/setDialog', payload });
    },
    [dispatch],
  );

  return setDialog;
};

export const useRemoveConversation = () => {
  const dispatch = useDispatch();

  const removeConversation = useCallback(
    (conversationIds: Array<string>, dialogId: string) => {
      return dispatch<any>({
        type: 'chatModel/removeConversation',
        payload: {
          dialog_id: dialogId,
          conversation_ids: conversationIds,
        },
      });
    },
    [dispatch],
  );

  return removeConversation;
};

/*
@deprecated
 */
export const useCompleteConversation = () => {
  const dispatch = useDispatch();

  const completeConversation = useCallback(
    (payload: any) => {
      return dispatch<any>({
        type: 'chatModel/completeConversation',
        payload,
      });
    },
    [dispatch],
  );

  return completeConversation;
};

// #region API provided for external calls

export const useCreateToken = (params: Record<string, any>) => {
  const dispatch = useDispatch();

  const createToken = useCallback(() => {
    return dispatch<any>({
      type: 'chatModel/createToken',
      payload: params,
    });
  }, [dispatch, params]);

  return createToken;
};

export const useCreateNextToken = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createToken'],
    mutationFn: async (params: Record<string, any>) => {
      const { data } = await chatService.createToken(params);
      if (data.retcode === 0) {
        queryClient.invalidateQueries({ queryKey: ['fetchTokenList'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, createToken: mutateAsync };
};

export const useFetchTokenList = (params: Record<string, any>) => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IToken[]>({
    queryKey: ['fetchTokenList', params],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await chatService.listToken(params);

      return data?.data ?? [];
    },
  });

  return { data, loading, refetch };
};

export const useRemoveNextToken = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['removeToken'],
    mutationFn: async (params: {
      tenantId: string;
      dialogId: string;
      tokens: string[];
    }) => {
      const { data } = await chatService.removeToken(params);
      if (data.retcode === 0) {
        queryClient.invalidateQueries({ queryKey: ['fetchTokenList'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, removeToken: mutateAsync };
};

type RangeValue = [Dayjs | null, Dayjs | null] | null;

const getDay = (date?: Dayjs) => date?.format('YYYY-MM-DD');

export const useFetchNextStats = () => {
  const [pickerValue, setPickerValue] = useState<RangeValue>([
    dayjs(),
    dayjs().subtract(7, 'day'),
  ]);
  const { data, isFetching: loading } = useQuery<IStats>({
    queryKey: ['fetchStats', pickerValue],
    initialData: {} as IStats,
    gcTime: 0,
    queryFn: async () => {
      if (Array.isArray(pickerValue) && pickerValue[0]) {
        const { data } = await chatService.getStats({
          fromDate: getDay(pickerValue[0]),
          toDate: getDay(pickerValue[1] ?? dayjs()),
        });
        return data?.data ?? {};
      }
      return {};
    },
  });

  return { data, loading, pickerValue, setPickerValue };
};

//#endregion

//#region shared chat

export const useCreateSharedConversation = () => {
  const dispatch = useDispatch();

  const createSharedConversation = useCallback(
    (userId?: string) => {
      return dispatch<any>({
        type: 'chatModel/createExternalConversation',
        payload: { userId },
      });
    },
    [dispatch],
  );

  return createSharedConversation;
};

export const useFetchSharedConversation = () => {
  const dispatch = useDispatch();

  const fetchSharedConversation = useCallback(
    (conversationId: string) => {
      return dispatch<any>({
        type: 'chatModel/getExternalConversation',
        payload: conversationId,
      });
    },
    [dispatch],
  );

  return fetchSharedConversation;
};

//#endregion
