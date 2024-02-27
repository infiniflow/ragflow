import showDeleteConfirm from '@/components/deleting-confirm';
import { MessageType } from '@/constants/chat';
import { fileIconMap } from '@/constants/common';
import { IConversation, IDialog } from '@/interfaces/database/chat';
import { getFileExtension } from '@/utils';
import omit from 'lodash/omit';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import { v4 as uuid } from 'uuid';
import { ChatSearchParams, EmptyConversationId } from './constants';
import {
  IClientConversation,
  IMessage,
  VariableTableDataType,
} from './interface';
import { ChatModelState } from './model';
import { isConversationIdNotExist } from './utils';

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

export const useGetChatSearchParams = () => {
  const [currentQueryParameters] = useSearchParams();

  return {
    dialogId: currentQueryParameters.get(ChatSearchParams.DialogId) || '',
    conversationId:
      currentQueryParameters.get(ChatSearchParams.ConversationId) || '',
  };
};

export const useSetCurrentConversation = () => {
  const dispatch = useDispatch();

  const setCurrentConversation = useCallback(
    (currentConversation: IClientConversation) => {
      dispatch({
        type: 'chatModel/setCurrentConversation',
        payload: currentConversation,
      });
    },
    [dispatch],
  );

  return setCurrentConversation;
};

export const useClickDialogCard = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();

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

export const useCreateTemporaryConversation = () => {
  const dispatch = useDispatch();
  const { dialogId } = useGetChatSearchParams();
  const { handleClickConversation } = useClickConversationCard();
  let chatModel = useSelector((state: any) => state.chatModel);

  const currentConversation: Pick<
    IClientConversation,
    'id' | 'message' | 'name' | 'dialog_id'
  > = chatModel.currentConversation;

  const conversationList: IClientConversation[] = chatModel.conversationList;
  const currentDialog: IDialog = chatModel.currentDialog;

  const setCurrentConversation = useSetCurrentConversation();

  const createTemporaryConversation = useCallback(() => {
    const firstConversation = conversationList[0];
    const messages = [...(firstConversation?.message ?? [])];
    if (messages.some((x) => x.id === EmptyConversationId)) {
      return;
    }
    messages.push({
      id: EmptyConversationId,
      content: currentDialog?.prompt_config?.prologue ?? '',
      role: MessageType.Assistant,
    });

    let nextCurrentConversation = currentConversation;

    // It’s the back-end data.
    if ('id' in currentConversation) {
      nextCurrentConversation = { ...currentConversation, message: messages };
    } else {
      // client data
      nextCurrentConversation = {
        id: EmptyConversationId,
        name: 'New conversation',
        dialog_id: dialogId,
        message: messages,
      };
    }

    const nextConversationList = [...conversationList];

    nextConversationList.unshift(
      nextCurrentConversation as IClientConversation,
    );

    setCurrentConversation(nextCurrentConversation as IClientConversation);

    dispatch({
      type: 'chatModel/setConversationList',
      payload: nextConversationList,
    });
    handleClickConversation(EmptyConversationId);
  }, [
    dispatch,
    currentConversation,
    dialogId,
    setCurrentConversation,
    handleClickConversation,
    conversationList,
    currentDialog,
  ]);

  return { createTemporaryConversation };
};

export const useFetchConversationList = () => {
  const dispatch = useDispatch();
  const conversationList: any[] = useSelector(
    (state: any) => state.chatModel.conversationList,
  );
  const { dialogId } = useGetChatSearchParams();

  const fetchConversationList = useCallback(async () => {
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

export const useSelectConversationList = () => {
  const [list, setList] = useState<Array<IConversation>>([]);
  let chatModel: ChatModelState = useSelector((state: any) => state.chatModel);
  const { conversationList, currentDialog } = chatModel;
  const { dialogId } = useGetChatSearchParams();
  const prologue = currentDialog?.prompt_config?.prologue ?? '';

  const addTemporaryConversation = useCallback(() => {
    setList(() => {
      const nextList = [
        {
          id: '',
          name: 'New conversation',
          dialog_id: dialogId,
          message: [
            {
              content: prologue,
              role: MessageType.Assistant,
            },
          ],
        } as IConversation,
        ...conversationList,
      ];
      return nextList;
    });
  }, [conversationList, dialogId, prologue]);

  useEffect(() => {
    addTemporaryConversation();
  }, [addTemporaryConversation]);

  return { list, addTemporaryConversation };
};

export const useClickConversationCard = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );

  const handleClickConversation = useCallback(
    (conversationId: string) => {
      newQueryParameters.set(ChatSearchParams.ConversationId, conversationId);
      setSearchParams(newQueryParameters);
    },
    [newQueryParameters, setSearchParams],
  );

  return { handleClickConversation };
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
  const setCurrentConversation = useSetCurrentConversation();

  const fetchConversation = useCallback(() => {
    if (isConversationIdNotExist(conversationId)) {
      dispatch<any>({
        type: 'chatModel/getConversation',
        payload: {
          conversation_id: conversationId,
        },
      });
    } else {
      setCurrentConversation({} as IClientConversation);
    }
  }, [dispatch, conversationId, setCurrentConversation]);

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
    if (conversationId !== '') {
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

export const useScrollToBottom = () => {
  const ref = useRef<HTMLDivElement>(null);
  let chatModel: ChatModelState = useSelector((state: any) => state.chatModel);
  const { currentConversation } = chatModel;

  const scrollToBottom = useCallback(() => {
    if (currentConversation.id) {
      ref.current?.scrollIntoView({ behavior: 'instant' });
    }
  }, [currentConversation]);

  useEffect(() => {
    scrollToBottom();
  }, [scrollToBottom]);

  return ref;
};

export const useGetFileIcon = () => {
  // const req = require.context('@/assets/svg/file-icon');
  // const ret = req.keys().map(req);
  // console.info(ret);
  // useEffect(() => {}, []);

  const getFileIcon = (filename: string) => {
    const ext: string = getFileExtension(filename);
    const iconPath = fileIconMap[ext as keyof typeof fileIconMap];
    // const x = require(`@/assets/svg/file-icon/${iconPath}`);
    return `@/assets/svg/file-icon/${iconPath}`;
  };

  return getFileIcon;
};

//#endregion
