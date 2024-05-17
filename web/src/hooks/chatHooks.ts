import {
  IConversation,
  IDialog,
  IStats,
  IToken,
} from '@/interfaces/database/chat';
import { useCallback } from 'react';
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

export const useCreateToken = (dialogId: string) => {
  const dispatch = useDispatch();

  const createToken = useCallback(() => {
    return dispatch<any>({
      type: 'chatModel/createToken',
      payload: { dialogId },
    });
  }, [dispatch, dialogId]);

  return createToken;
};

export const useListToken = () => {
  const dispatch = useDispatch();

  const listToken = useCallback(
    (dialogId: string) => {
      return dispatch<any>({
        type: 'chatModel/listToken',
        payload: { dialogId },
      });
    },
    [dispatch],
  );

  return listToken;
};

export const useSelectTokenList = () => {
  const tokenList: IToken[] = useSelector(
    (state: any) => state.chatModel.tokenList,
  );

  return tokenList;
};

export const useRemoveToken = () => {
  const dispatch = useDispatch();

  const removeToken = useCallback(
    (payload: { tenantId: string; dialogId: string; tokens: string[] }) => {
      return dispatch<any>({
        type: 'chatModel/removeToken',
        payload: payload,
      });
    },
    [dispatch],
  );

  return removeToken;
};

export const useFetchStats = () => {
  const dispatch = useDispatch();

  const fetchStats = useCallback(
    (payload: any) => {
      return dispatch<any>({
        type: 'chatModel/getStats',
        payload,
      });
    },
    [dispatch],
  );

  return fetchStats;
};

export const useSelectStats = () => {
  const stats: IStats = useSelector((state: any) => state.chatModel.stats);

  return stats;
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
