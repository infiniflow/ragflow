import { IConversation, IDialog } from '@/interfaces/database/chat';
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
