import { MessageType } from '@/constants/chat';
import { fileIconMap } from '@/constants/common';
import { useSetModalState, useShowDeleteConfirm } from '@/hooks/commonHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { IConversation, IDialog } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { getFileExtension } from '@/utils';
import omit from 'lodash/omit';
import {
  ChangeEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import { v4 as uuid } from 'uuid';
import { ChatSearchParams } from './constants';
import {
  IClientConversation,
  IMessage,
  VariableTableDataType,
} from './interface';
import { ChatModelState } from './model';
import { isConversationIdExist } from './utils';

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

export const useSelectCurrentDialog = () => {
  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  return currentDialog;
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

export const useFetchDialogOnMount = (
  dialogId: string,
  visible: boolean,
): IDialog => {
  const currentDialog: IDialog = useSelectCurrentDialog();
  const fetchDialog = useFetchDialog();

  useEffect(() => {
    if (dialogId && visible) {
      fetchDialog(dialogId);
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
      dispatch({
        type: 'chatModel/setCurrentDialog',
        payload: { id: dialogId },
      });
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
  const showDeleteConfirm = useShowDeleteConfirm();

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

export const useHandleItemHover = () => {
  const [activated, setActivated] = useState<string>('');

  const handleItemEnter = (id: string) => {
    setActivated(id);
  };

  const handleItemLeave = () => {
    setActivated('');
  };

  return {
    activated,
    handleItemEnter,
    handleItemLeave,
  };
};

export const useEditDialog = () => {
  const [dialog, setDialog] = useState<IDialog>({} as IDialog);
  const fetchDialog = useFetchDialog();
  const submitDialog = useSetDialog();
  const loading = useOneNamespaceEffectsLoading('chatModel', ['setDialog']);

  const {
    visible: dialogEditVisible,
    hideModal: hideDialogEditModal,
    showModal: showDialogEditModal,
  } = useSetModalState();

  const onDialogEditOk = useCallback(
    async (dialog: IDialog) => {
      const ret = await submitDialog(dialog);

      if (ret === 0) {
        hideDialogEditModal();
      }
    },
    [submitDialog, hideDialogEditModal],
  );

  const handleShowDialogEditModal = useCallback(
    async (dialogId?: string) => {
      if (dialogId) {
        const ret = await fetchDialog(dialogId, false);
        if (ret.retcode === 0) {
          setDialog(ret.data);
        }
      }
      showDialogEditModal();
    },
    [showDialogEditModal, fetchDialog],
  );

  const clearDialog = useCallback(() => {
    setDialog({} as IDialog);
  }, []);

  return {
    dialogSettingLoading: loading,
    initialDialog: dialog,
    onDialogEditOk,
    dialogEditVisible,
    hideDialogEditModal,
    showDialogEditModal: handleShowDialogEditModal,
    clearDialog,
  };
};

//#region conversation

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
    setList((pre) => {
      if (dialogId) {
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
      }

      return pre;
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

  const setConversation = useCallback(
    (message: string) => {
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
    },
    [dispatch, dialogId],
  );

  return { setConversation };
};

export const useSelectCurrentConversation = () => {
  const [currentConversation, setCurrentConversation] =
    useState<IClientConversation>({} as IClientConversation);

  const conversation: IClientConversation = useSelector(
    (state: any) => state.chatModel.currentConversation,
  );
  const dialog = useSelectCurrentDialog();
  const { conversationId, dialogId } = useGetChatSearchParams();

  const addNewestConversation = useCallback((message: string) => {
    setCurrentConversation((pre) => {
      return {
        ...pre,
        message: [
          ...pre.message,
          {
            role: MessageType.User,
            content: message,
            id: uuid(),
          } as IMessage,
          {
            role: MessageType.Assistant,
            content: '',
            id: uuid(),
            reference: [],
          } as IMessage,
        ],
      };
    });
  }, []);

  const removeLatestMessage = useCallback(() => {
    setCurrentConversation((pre) => {
      const nextMessages = pre.message.slice(0, -2);
      return {
        ...pre,
        message: nextMessages,
      };
    });
  }, []);

  const addPrologue = useCallback(() => {
    if (dialogId !== '' && conversationId === '') {
      const prologue = dialog.prompt_config?.prologue;

      const nextMessage = {
        role: MessageType.Assistant,
        content: prologue,
        id: uuid(),
      } as IMessage;

      setCurrentConversation({
        id: '',
        dialog_id: dialogId,
        reference: [],
        message: [nextMessage],
      } as any);
    }
  }, [conversationId, dialog, dialogId]);

  useEffect(() => {
    addPrologue();
  }, [addPrologue]);

  useEffect(() => {
    setCurrentConversation(conversation);
  }, [conversation]);

  return { currentConversation, addNewestConversation, removeLatestMessage };
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

export const useScrollToBottom = (currentConversation: IClientConversation) => {
  const ref = useRef<HTMLDivElement>(null);

  const scrollToBottom = useCallback(() => {
    console.info('useScrollToBottom');
    if (currentConversation.id) {
      ref.current?.scrollIntoView({ behavior: 'instant' });
    }
  }, [currentConversation]);

  useEffect(() => {
    scrollToBottom();
  }, [scrollToBottom]);

  return ref;
};

export const useFetchConversationOnMount = () => {
  const { conversationId } = useGetChatSearchParams();
  const fetchConversation = useFetchConversation();
  const { currentConversation, addNewestConversation, removeLatestMessage } =
    useSelectCurrentConversation();
  const ref = useScrollToBottom(currentConversation);

  const fetchConversationOnMount = useCallback(() => {
    if (isConversationIdExist(conversationId)) {
      fetchConversation(conversationId);
    }
  }, [fetchConversation, conversationId]);

  useEffect(() => {
    fetchConversationOnMount();
  }, [fetchConversationOnMount]);

  return {
    currentConversation,
    addNewestConversation,
    ref,
    removeLatestMessage,
  };
};

export const useHandleMessageInputChange = () => {
  const [value, setValue] = useState('');

  const handleInputChange: ChangeEventHandler<HTMLInputElement> = (e) => {
    const value = e.target.value;
    const nextValue = value.replaceAll('\\n', '\n').replaceAll('\\t', '\t');
    setValue(nextValue);
  };

  return {
    handleInputChange,
    value,
    setValue,
  };
};

export const useSendMessage = (
  conversation: IClientConversation,
  addNewestConversation: (message: string) => void,
  removeLatestMessage: () => void,
) => {
  const loading = useOneNamespaceEffectsLoading('chatModel', [
    'completeConversation',
  ]);
  const dispatch = useDispatch();
  const { setConversation } = useSetConversation();
  const { conversationId } = useGetChatSearchParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  // const conversation: IClientConversation = useSelector(
  //   (state: any) => state.chatModel.currentConversation,
  // );
  const fetchConversation = useFetchConversation();

  const { handleClickConversation } = useClickConversationCard();

  const sendMessage = useCallback(
    async (message: string, id?: string) => {
      const retcode = await dispatch<any>({
        type: 'chatModel/completeConversation',
        payload: {
          conversation_id: id ?? conversationId,
          messages: [
            ...(conversation?.message ?? []).map((x: IMessage) =>
              omit(x, 'id'),
            ),
            {
              role: MessageType.User,
              content: message,
            },
          ],
        },
      });

      if (retcode === 0) {
        if (id) {
          // new conversation
          handleClickConversation(id);
        } else {
          fetchConversation(conversationId);
        }
      } else {
        // cancel loading
        setValue(message);
        removeLatestMessage();
      }
    },
    [
      dispatch,
      conversation?.message,
      conversationId,
      fetchConversation,
      handleClickConversation,
      removeLatestMessage,
      setValue,
    ],
  );

  const handleSendMessage = useCallback(
    async (message: string) => {
      if (conversationId !== '') {
        sendMessage(message);
      } else {
        const data = await setConversation(message);
        if (data.retcode === 0) {
          const id = data.data.id;
          sendMessage(message, id);
        }
      }
    },
    [conversationId, setConversation, sendMessage],
  );

  const handlePressEnter = () => {
    if (!loading) {
      setValue('');
      addNewestConversation(value);
      handleSendMessage(value.trim());
    }
  };

  return {
    handlePressEnter,
    handleInputChange,
    value,
    loading,
  };
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

export const useRemoveConversation = () => {
  const dispatch = useDispatch();
  const { dialogId } = useGetChatSearchParams();
  const { handleClickConversation } = useClickConversationCard();
  const showDeleteConfirm = useShowDeleteConfirm();

  const removeConversation = (conversationIds: Array<string>) => async () => {
    const ret = await dispatch<any>({
      type: 'chatModel/removeConversation',
      payload: {
        dialog_id: dialogId,
        conversation_ids: conversationIds,
      },
    });

    if (ret === 0) {
      handleClickConversation('');
    }

    return ret;
  };

  const onRemoveConversation = (conversationIds: Array<string>) => {
    showDeleteConfirm({ onOk: removeConversation(conversationIds) });
  };

  return { onRemoveConversation };
};

export const useRenameConversation = () => {
  const dispatch = useDispatch();
  const [conversation, setConversation] = useState<IClientConversation>(
    {} as IClientConversation,
  );
  const fetchConversation = useFetchConversation();
  const {
    visible: conversationRenameVisible,
    hideModal: hideConversationRenameModal,
    showModal: showConversationRenameModal,
  } = useSetModalState();

  const onConversationRenameOk = useCallback(
    async (name: string) => {
      const ret = await dispatch<any>({
        type: 'chatModel/setConversation',
        payload: { ...conversation, conversation_id: conversation.id, name },
      });

      if (ret.retcode === 0) {
        hideConversationRenameModal();
      }
    },
    [dispatch, conversation, hideConversationRenameModal],
  );

  const loading = useOneNamespaceEffectsLoading('chatModel', [
    'setConversation',
  ]);

  const handleShowConversationRenameModal = useCallback(
    async (conversationId: string) => {
      const ret = await fetchConversation(conversationId, false);
      if (ret.retcode === 0) {
        setConversation(ret.data);
      }
      showConversationRenameModal();
    },
    [showConversationRenameModal, fetchConversation],
  );

  return {
    conversationRenameLoading: loading,
    initialConversationName: conversation.name,
    onConversationRenameOk,
    conversationRenameVisible,
    hideConversationRenameModal,
    showConversationRenameModal: handleShowConversationRenameModal,
  };
};

export const useClickDrawer = () => {
  const { visible, showModal, hideModal } = useSetModalState();
  const [selectedChunk, setSelectedChunk] = useState<IChunk>({} as IChunk);
  const [documentId, setDocumentId] = useState<string>('');

  const clickDocumentButton = useCallback(
    (documentId: string, chunk: IChunk) => {
      showModal();
      setSelectedChunk(chunk);
      setDocumentId(documentId);
    },
    [showModal],
  );

  return {
    clickDocumentButton,
    visible,
    showModal,
    hideModal,
    selectedChunk,
    documentId,
  };
};

export const useSelectDialogListLoading = () => {
  return useOneNamespaceEffectsLoading('chatModel', ['listDialog']);
};
export const useSelectConversationListLoading = () => {
  return useOneNamespaceEffectsLoading('chatModel', ['listConversation']);
};
export const useSelectConversationLoading = () => {
  return useOneNamespaceEffectsLoading('chatModel', ['getConversation']);
};

export const useGetSendButtonDisabled = () => {
  const { dialogId, conversationId } = useGetChatSearchParams();

  return dialogId === '' && conversationId === '';
};
//#endregion
