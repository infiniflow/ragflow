import { MessageType } from '@/constants/chat';
import { fileIconMap } from '@/constants/common';
import {
  useFetchManualConversation,
  useFetchManualDialog,
  useFetchNextConversation,
  useFetchNextConversationList,
  useFetchNextDialog,
  useGetChatSearchParams,
  useRemoveNextConversation,
  useRemoveNextDialog,
  useSetNextDialog,
  useUpdateNextConversation,
} from '@/hooks/chat-hooks';
import {
  useSetModalState,
  useShowDeleteConfirm,
  useTranslate,
} from '@/hooks/common-hooks';
import { useSendMessageWithSse } from '@/hooks/logic-hooks';
import {
  IAnswer,
  IConversation,
  IDialog,
  Message,
} from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { getFileExtension } from '@/utils';
import { useMutationState } from '@tanstack/react-query';
import { get } from 'lodash';
import omit from 'lodash/omit';
import trim from 'lodash/trim';
import {
  ChangeEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useSearchParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { ChatSearchParams } from './constants';
import {
  IClientConversation,
  IMessage,
  VariableTableDataType,
} from './interface';

export const useSelectCurrentDialog = () => {
  const data = useMutationState({
    filters: { mutationKey: ['fetchDialog'] },
    select: (mutation) => {
      return get(mutation, 'state.data.data', {});
    },
  });

  return (data.at(-1) ?? {}) as IDialog;
};

export const useSelectPromptConfigParameters = (): VariableTableDataType[] => {
  const { data: currentDialog } = useFetchNextDialog();

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

export const useDeleteDialog = () => {
  const showDeleteConfirm = useShowDeleteConfirm();

  const { removeDialog } = useRemoveNextDialog();

  const onRemoveDialog = (dialogIds: Array<string>) => {
    showDeleteConfirm({ onOk: () => removeDialog(dialogIds) });
  };

  return { onRemoveDialog };
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
  const { fetchDialog } = useFetchManualDialog();
  const { setDialog: submitDialog, loading } = useSetNextDialog();

  const {
    visible: dialogEditVisible,
    hideModal: hideDialogEditModal,
    showModal: showDialogEditModal,
  } = useSetModalState();

  const hideModal = useCallback(() => {
    setDialog({} as IDialog);
    hideDialogEditModal();
  }, [hideDialogEditModal]);

  const onDialogEditOk = useCallback(
    async (dialog: IDialog) => {
      const ret = await submitDialog(dialog);

      if (ret === 0) {
        hideModal();
      }
    },
    [submitDialog, hideModal],
  );

  const handleShowDialogEditModal = useCallback(
    async (dialogId?: string) => {
      if (dialogId) {
        const ret = await fetchDialog(dialogId);
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
    hideDialogEditModal: hideModal,
    showDialogEditModal: handleShowDialogEditModal,
    clearDialog,
  };
};

//#region conversation

export const useSelectDerivedConversationList = () => {
  const { t } = useTranslate('chat');

  const [list, setList] = useState<Array<IConversation>>([]);
  const { data: currentDialog } = useFetchNextDialog();
  const { data: conversationList, loading } = useFetchNextConversationList();
  const { dialogId } = useGetChatSearchParams();
  const prologue = currentDialog?.prompt_config?.prologue ?? '';

  const addTemporaryConversation = useCallback(() => {
    setList((pre) => {
      if (dialogId) {
        const nextList = [
          {
            id: '',
            name: t('newConversation'),
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
  }, [conversationList, dialogId, prologue, t]);

  useEffect(() => {
    addTemporaryConversation();
  }, [addTemporaryConversation]);

  return { list, addTemporaryConversation, loading };
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
  const { dialogId } = useGetChatSearchParams();
  const { updateConversation } = useUpdateNextConversation();

  const setConversation = useCallback(
    (message: string) => {
      return updateConversation({
        dialog_id: dialogId,
        name: message,
        message: [
          {
            role: MessageType.Assistant,
            content: message,
          },
        ],
      });
    },
    [updateConversation, dialogId],
  );

  return { setConversation };
};

export const useSelectCurrentConversation = () => {
  const [currentConversation, setCurrentConversation] =
    useState<IClientConversation>({} as IClientConversation);
  const { data: conversation, loading } = useFetchNextConversation();
  const { data: dialog } = useFetchNextDialog();
  const { conversationId, dialogId } = useGetChatSearchParams();

  const addNewestConversation = useCallback(
    (message: Partial<Message>, answer: string = '') => {
      setCurrentConversation((pre) => {
        return {
          ...pre,
          message: [
            ...pre.message,
            {
              role: MessageType.User,
              content: message.content,
              doc_ids: message.doc_ids,
              id: uuid(),
            } as IMessage,
            {
              role: MessageType.Assistant,
              content: answer,
              id: uuid(),
              reference: {},
            } as IMessage,
          ],
        };
      });
    },
    [],
  );

  const addNewestAnswer = useCallback((answer: IAnswer) => {
    setCurrentConversation((pre) => {
      const latestMessage = pre.message?.at(-1);

      if (latestMessage) {
        return {
          ...pre,
          message: [
            ...pre.message.slice(0, -1),
            {
              ...latestMessage,
              content: answer.answer,
              reference: answer.reference,
            } as IMessage,
          ],
        };
      }
      return pre;
    });
  }, []);

  const removeLatestMessage = useCallback(() => {
    setCurrentConversation((pre) => {
      const nextMessages = pre.message?.slice(0, -2) ?? [];
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
    if (conversationId) {
      setCurrentConversation(conversation);
    }
  }, [conversation, conversationId]);

  return {
    currentConversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
    loading,
  };
};

export const useScrollToBottom = (currentConversation: IClientConversation) => {
  const ref = useRef<HTMLDivElement>(null);

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

export const useFetchConversationOnMount = () => {
  const { conversationId } = useGetChatSearchParams();
  const {
    currentConversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
    loading,
  } = useSelectCurrentConversation();
  const ref = useScrollToBottom(currentConversation);

  return {
    currentConversation,
    addNewestConversation,
    ref,
    removeLatestMessage,
    addNewestAnswer,
    conversationId,
    loading,
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
  addNewestConversation: (message: Partial<Message>, answer?: string) => void,
  removeLatestMessage: () => void,
  addNewestAnswer: (answer: IAnswer) => void,
) => {
  const { setConversation } = useSetConversation();
  const { conversationId } = useGetChatSearchParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { handleClickConversation } = useClickConversationCard();
  const { send, answer, done, setDone } = useSendMessageWithSse();

  const sendMessage = useCallback(
    async (message: string, documentIds: string[], id?: string) => {
      const res = await send({
        conversation_id: id ?? conversationId,
        messages: [
          ...(conversation?.message ?? []).map((x: IMessage) => omit(x, 'id')),
          {
            id: uuid(),
            role: MessageType.User,
            content: message,
            doc_ids: documentIds,
          },
        ],
      });

      if (res && (res?.response.status !== 200 || res?.data?.retcode !== 0)) {
        // cancel loading
        setValue(message);
        console.info('removeLatestMessage111');
        removeLatestMessage();
      } else {
        if (id) {
          console.info('111');
          // new conversation
          handleClickConversation(id);
        } else {
          console.info('222');
          // fetchConversation(conversationId);
        }
      }
    },
    [
      conversation?.message,
      conversationId,
      handleClickConversation,
      removeLatestMessage,
      setValue,
      send,
    ],
  );

  const handleSendMessage = useCallback(
    async (message: string, documentIds: string[]) => {
      if (conversationId !== '') {
        sendMessage(message, documentIds);
      } else {
        const data = await setConversation(message);
        if (data.retcode === 0) {
          const id = data.data.id;
          sendMessage(message, documentIds, id);
        }
      }
    },
    [conversationId, setConversation, sendMessage],
  );

  useEffect(() => {
    //  #1289
    if (answer.answer && answer?.conversationId === conversationId) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer, conversationId]);

  useEffect(() => {
    // #1289 switch to another conversion window when the last conversion answer doesn't finish.
    if (conversationId) {
      setDone(true);
    }
  }, [setDone, conversationId]);

  const handlePressEnter = useCallback(
    (documentIds: string[]) => {
      if (trim(value) === '') return;

      addNewestConversation({ content: value, doc_ids: documentIds });
      if (done) {
        setValue('');
        handleSendMessage(value.trim(), documentIds);
      }
    },
    [addNewestConversation, handleSendMessage, done, setValue, value],
  );

  return {
    handlePressEnter,
    handleInputChange,
    value,
    setValue,
    loading: !done,
  };
};

export const useGetFileIcon = () => {
  const getFileIcon = (filename: string) => {
    const ext: string = getFileExtension(filename);
    const iconPath = fileIconMap[ext as keyof typeof fileIconMap];
    return `@/assets/svg/file-icon/${iconPath}`;
  };

  return getFileIcon;
};

export const useDeleteConversation = () => {
  const { handleClickConversation } = useClickConversationCard();
  const showDeleteConfirm = useShowDeleteConfirm();
  const { removeConversation } = useRemoveNextConversation();

  const deleteConversation = (conversationIds: Array<string>) => async () => {
    const ret = await removeConversation(conversationIds);
    if (ret === 0) {
      handleClickConversation('');
    }
    return ret;
  };

  const onRemoveConversation = (conversationIds: Array<string>) => {
    showDeleteConfirm({ onOk: deleteConversation(conversationIds) });
  };

  return { onRemoveConversation };
};

export const useRenameConversation = () => {
  const [conversation, setConversation] = useState<IClientConversation>(
    {} as IClientConversation,
  );
  const { fetchConversation } = useFetchManualConversation();
  const {
    visible: conversationRenameVisible,
    hideModal: hideConversationRenameModal,
    showModal: showConversationRenameModal,
  } = useSetModalState();
  const { updateConversation, loading } = useUpdateNextConversation();

  const onConversationRenameOk = useCallback(
    async (name: string) => {
      const ret = await updateConversation({
        ...conversation,
        conversation_id: conversation.id,
        name,
      });

      if (ret.retcode === 0) {
        hideConversationRenameModal();
      }
    },
    [updateConversation, conversation, hideConversationRenameModal],
  );

  const handleShowConversationRenameModal = useCallback(
    async (conversationId: string) => {
      const ret = await fetchConversation(conversationId);
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

export const useGetSendButtonDisabled = () => {
  const { dialogId, conversationId } = useGetChatSearchParams();

  return dialogId === '' && conversationId === '';
};

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

export const useCreateConversationBeforeUploadDocument = () => {
  const { setConversation } = useSetConversation();
  const { dialogId } = useGetChatSearchParams();

  const { handleClickConversation } = useClickConversationCard();

  const createConversationBeforeUploadDocument = useCallback(
    async (message: string) => {
      const data = await setConversation(message);
      if (data.retcode === 0) {
        const id = data.data.id;
        handleClickConversation(id);
      }
      return data;
    },
    [setConversation, handleClickConversation],
  );

  return {
    createConversationBeforeUploadDocument,
    dialogId,
  };
};
//#endregion
