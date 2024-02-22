import showDeleteConfirm from '@/components/deleting-confirm';
import { MessageType } from '@/constants/chat';
import { IDialog } from '@/interfaces/database/chat';
import omit from 'lodash/omit';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import { v4 as uuid } from 'uuid';
import { ChatSearchParams, EmptyConversationId } from './constants';
import {
  IClientConversation,
  IMessage,
  VariableTableDataType,
} from './interface';

export const useFetchDialogList = () => {
  const dispatch = useDispatch();
  const dialogList: IDialog[] = useSelector(
    (state: any) => state.chatModel.dialogList,
  );

  useEffect(() => {
    dispatch({ type: 'chatModel/listDialog' });
  }, [dispatch]);

  return dialogList;
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

export const useFetchDialog = (dialogId: string, visible: boolean): IDialog => {
  const dispatch = useDispatch();
  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  const fetchDialog = useCallback(() => {
    if (dialogId) {
      dispatch({
        type: 'chatModel/getDialog',
        payload: { dialog_id: dialogId },
      });
    }
  }, [dispatch, dialogId]);

  useEffect(() => {
    if (dialogId && visible) {
      fetchDialog();
    }
  }, [dialogId, fetchDialog, visible]);

  return currentDialog;
};

export const useSetCurrentDialog = () => {
  const dispatch = useDispatch();

  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  const setCurrentDialog = useCallback(
    (dialogId: string) => {
      if (dialogId) {
        dispatch({
          type: 'chatModel/setCurrentDialog',
          payload: { id: dialogId },
        });
      }
    },
    [dispatch],
  );

  return { currentDialog, setCurrentDialog };
};

export const useResetCurrentDialog = () => {
  const dispatch = useDispatch();

  const resetCurrentDialog = useCallback(() => {
    dispatch({
      type: 'chatModel/setCurrentDialog',
      payload: {},
    });
  }, [dispatch]);

  return { resetCurrentDialog };
};

export const useSelectPromptConfigParameters = (): VariableTableDataType[] => {
  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  const finalParameters: VariableTableDataType[] = useMemo(() => {
    const parameters = currentDialog?.prompt_config?.parameters ?? [];
    if (!currentDialog.id) {
      // The newly created chat has a default parameter
      return [{ key: uuid(), variable: 'knowledge', optional: false }];
    }
    return parameters.map((x) => ({
      key: uuid(),
      variable: x.key,
      optional: x.optional,
    }));
  }, [currentDialog]);

  return finalParameters;
};

export const useRemoveDialog = () => {
  const dispatch = useDispatch();

  const removeDocument = (dialogIds: Array<string>) => () => {
    return dispatch({
      type: 'chatModel/removeDialog',
      payload: {
        dialog_ids: dialogIds,
      },
    });
  };

  const onRemoveDialog = (dialogIds: Array<string>) => {
    showDeleteConfirm({ onOk: removeDocument(dialogIds) });
  };

  return { onRemoveDialog };
};

export const useClickDialogCard = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();

  const newQueryParameters: URLSearchParams = useMemo(() => {
    return new URLSearchParams(currentQueryParameters.toString());
  }, [currentQueryParameters]);

  const handleClickDialog = useCallback(
    (dialogId: string) => {
      newQueryParameters.set(ChatSearchParams.DialogId, dialogId);
      setSearchParams(newQueryParameters);
    },
    [newQueryParameters, setSearchParams],
  );

  return { handleClickDialog };
};

export const useGetChatSearchParams = () => {
  const [currentQueryParameters] = useSearchParams();

  return {
    dialogId: currentQueryParameters.get(ChatSearchParams.DialogId) || '',
    conversationId:
      currentQueryParameters.get(ChatSearchParams.ConversationId) || '',
  };
};

export const useSelectFirstDialogOnMount = () => {
  const dialogList = useFetchDialogList();
  const { dialogId } = useGetChatSearchParams();

  const { handleClickDialog } = useClickDialogCard();

  useEffect(() => {
    if (dialogList.length > 0 && !dialogId) {
      handleClickDialog(dialogList[0].id);
    }
  }, [dialogList, handleClickDialog, dialogId]);

  return dialogList;
};

//#region conversation

export const useFetchConversationList = (dialogId?: string) => {
  const dispatch = useDispatch();
  const conversationList: any[] = useSelector(
    (state: any) => state.chatModel.conversationList,
  );

  const fetchConversationList = useCallback(() => {
    if (dialogId) {
      dispatch({
        type: 'chatModel/listConversation',
        payload: { dialog_id: dialogId },
      });
    }
  }, [dispatch, dialogId]);

  useEffect(() => {
    fetchConversationList();
  }, [fetchConversationList]);

  return conversationList;
};

export const useClickConversationCard = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const newQueryParameters: URLSearchParams = new URLSearchParams(
    currentQueryParameters.toString(),
  );

  const handleClickConversation = (conversationId: string) => {
    newQueryParameters.set(ChatSearchParams.ConversationId, conversationId);
    setSearchParams(newQueryParameters);
  };

  return { handleClickConversation };
};

export const useCreateTemporaryConversation = () => {
  const dispatch = useDispatch();
  const { dialogId } = useGetChatSearchParams();
  const { handleClickConversation } = useClickConversationCard();
  let chatModel = useSelector((state: any) => state.chatModel);
  let currentConversation: Pick<
    IClientConversation,
    'id' | 'message' | 'name' | 'dialog_id'
  > = chatModel.currentConversation;
  let conversationList: IClientConversation[] = chatModel.conversationList;

  const createTemporaryConversation = (message: string) => {
    const messages = [...(currentConversation?.message ?? [])];
    if (messages.some((x) => x.id === EmptyConversationId)) {
      return;
    }
    messages.unshift({
      id: EmptyConversationId,
      content: message,
      role: MessageType.Assistant,
    });

    // It’s the back-end data.
    if ('id' in currentConversation) {
      currentConversation = { ...currentConversation, message: messages };
    } else {
      // client data
      currentConversation = {
        id: EmptyConversationId,
        name: 'New conversation',
        dialog_id: dialogId,
        message: messages,
      };
    }

    const nextConversationList = [...conversationList];

    nextConversationList.push(currentConversation as IClientConversation);

    dispatch({
      type: 'chatModel/setCurrentConversation',
      payload: currentConversation,
    });

    dispatch({
      type: 'chatModel/setConversationList',
      payload: nextConversationList,
    });
    handleClickConversation(EmptyConversationId);
  };

  return { createTemporaryConversation };
};

export const useSetConversation = () => {
  const dispatch = useDispatch();
  const { dialogId } = useGetChatSearchParams();

  const setConversation = (message: string) => {
    return dispatch<any>({
      type: 'chatModel/setConversation',
      payload: {
        // conversation_id: '',
        dialog_id: dialogId,
        name: message,
        message: [
          {
            role: MessageType.Assistant,
            content: message,
          },
        ],
      },
    });
  };

  return { setConversation };
};

export const useFetchConversation = () => {
  const dispatch = useDispatch();
  const { conversationId } = useGetChatSearchParams();
  const conversation = useSelector(
    (state: any) => state.chatModel.currentConversation,
  );

  const fetchConversation = useCallback(() => {
    if (conversationId !== EmptyConversationId && conversationId !== '') {
      dispatch({
        type: 'chatModel/getConversation',
        payload: {
          conversation_id: conversationId,
        },
      });
    }
  }, [dispatch, conversationId]);

  useEffect(() => {
    fetchConversation();
  }, [fetchConversation]);

  return conversation;
};

export const useSendMessage = () => {
  const dispatch = useDispatch();
  const { setConversation } = useSetConversation();
  const { conversationId } = useGetChatSearchParams();
  const conversation = useSelector(
    (state: any) => state.chatModel.currentConversation,
  );
  const { handleClickConversation } = useClickConversationCard();

  const sendMessage = (message: string, id?: string) => {
    dispatch({
      type: 'chatModel/completeConversation',
      payload: {
        conversation_id: id ?? conversationId,
        messages: [
          ...(conversation?.message ?? []).map((x: IMessage) => omit(x, 'id')),
          {
            role: MessageType.User,
            content: message,
          },
        ],
      },
    });
  };

  const handleSendMessage = async (message: string) => {
    if (conversationId !== EmptyConversationId) {
      sendMessage(message);
    } else {
      const data = await setConversation(message);
      if (data.retcode === 0) {
        const id = data.data.id;
        handleClickConversation(id);
        sendMessage(message, id);
      }
    }
  };

  return { sendMessage: handleSendMessage };
};

//#endregion
