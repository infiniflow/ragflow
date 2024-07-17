import { MessageType } from '@/constants/chat';
import { fileIconMap } from '@/constants/common';
import {
  useCreateToken,
  useFetchConversation,
  useFetchConversationList,
  useFetchDialog,
  useFetchDialogList,
  useFetchStats,
  useListToken,
  useRemoveConversation,
  useRemoveDialog,
  useRemoveToken,
  useSelectConversationList,
  useSelectDialogList,
  useSelectStats,
  useSelectTokenList,
  useSetDialog,
  useUpdateConversation,
} from '@/hooks/chat-hooks';
import {
  useSetModalState,
  useShowDeleteConfirm,
  useTranslate,
} from '@/hooks/common-hooks';
import { useSendMessageWithSse } from '@/hooks/logic-hooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/store-hooks';
import {
  IAnswer,
  IConversation,
  IDialog,
  IStats,
} from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { getFileExtension } from '@/utils';
import { message } from 'antd';
import dayjs, { Dayjs } from 'dayjs';
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

export const useSelectCurrentDialog = () => {
  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  return currentDialog;
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

export const useDeleteDialog = () => {
  const showDeleteConfirm = useShowDeleteConfirm();

  const removeDocument = useRemoveDialog();

  const onRemoveDialog = (dialogIds: Array<string>) => {
    showDeleteConfirm({ onOk: () => removeDocument(dialogIds) });
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
  const fetchDialogList = useFetchDialogList();
  const dialogList = useSelectDialogList();

  const { handleClickDialog } = useClickDialogCard();

  const fetchList = useCallback(async () => {
    const data = await fetchDialogList();
    if (data.retcode === 0 && data.data.length > 0) {
      handleClickDialog(data.data[0].id);
    }
  }, [fetchDialogList, handleClickDialog]);

  useEffect(() => {
    fetchList();
  }, [fetchList]);

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

export const useFetchConversationListOnMount = () => {
  const conversationList = useSelectConversationList();
  const { dialogId } = useGetChatSearchParams();
  const fetchConversationList = useFetchConversationList();

  useEffect(() => {
    fetchConversationList(dialogId);
  }, [fetchConversationList, dialogId]);

  return conversationList;
};

export const useSelectDerivedConversationList = () => {
  const [list, setList] = useState<Array<IConversation>>([]);
  let chatModel: ChatModelState = useSelector((state: any) => state.chatModel);
  const { conversationList, currentDialog } = chatModel;
  const { dialogId } = useGetChatSearchParams();
  const prologue = currentDialog?.prompt_config?.prologue ?? '';
  const { t } = useTranslate('chat');
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
  const { dialogId } = useGetChatSearchParams();
  const updateConversation = useUpdateConversation();

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

  const conversation: IClientConversation = useSelector(
    (state: any) => state.chatModel.currentConversation,
  );
  const dialog = useSelectCurrentDialog();
  const { conversationId, dialogId } = useGetChatSearchParams();

  const addNewestConversation = useCallback(
    (message: string, answer: string = '') => {
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
  const fetchConversation = useFetchConversation();
  const {
    currentConversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
  } = useSelectCurrentConversation();
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
    addNewestAnswer,
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
  addNewestConversation: (message: string, answer?: string) => void,
  removeLatestMessage: () => void,
  addNewestAnswer: (answer: IAnswer) => void,
) => {
  const { setConversation } = useSetConversation();
  const { conversationId } = useGetChatSearchParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { handleClickConversation } = useClickConversationCard();
  const { send, answer, done, setDone } = useSendMessageWithSse();

  const sendMessage = useCallback(
    async (message: string, id?: string) => {
      const res = await send({
        conversation_id: id ?? conversationId,
        messages: [
          ...(conversation?.message ?? []).map((x: IMessage) => omit(x, 'id')),
          {
            role: MessageType.User,
            content: message,
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

  const handlePressEnter = useCallback(() => {
    if (trim(value) === '') return;

    if (done) {
      setValue('');
      handleSendMessage(value.trim());
    }
    addNewestConversation(value);
  }, [addNewestConversation, handleSendMessage, done, setValue, value]);

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
  const { dialogId } = useGetChatSearchParams();
  const { handleClickConversation } = useClickConversationCard();
  const showDeleteConfirm = useShowDeleteConfirm();
  const removeConversation = useRemoveConversation();

  const deleteConversation = (conversationIds: Array<string>) => async () => {
    const ret = await removeConversation(conversationIds, dialogId);
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
  const fetchConversation = useFetchConversation();
  const {
    visible: conversationRenameVisible,
    hideModal: hideConversationRenameModal,
    showModal: showConversationRenameModal,
  } = useSetModalState();
  const updateConversation = useUpdateConversation();

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

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};
//#endregion

//#region API provided for external calls

type RangeValue = [Dayjs | null, Dayjs | null] | null;

const getDay = (date: Dayjs) => date.format('YYYY-MM-DD');

export const useFetchStatsOnMount = (visible: boolean) => {
  const fetchStats = useFetchStats();
  const [pickerValue, setPickerValue] = useState<RangeValue>([
    dayjs(),
    dayjs().subtract(7, 'day'),
  ]);

  useEffect(() => {
    if (visible && Array.isArray(pickerValue) && pickerValue[0]) {
      fetchStats({
        fromDate: getDay(pickerValue[0]),
        toDate: getDay(pickerValue[1] ?? dayjs()),
      });
    }
  }, [fetchStats, pickerValue, visible]);

  return {
    pickerValue,
    setPickerValue,
  };
};

export const useOperateApiKey = (visible: boolean, dialogId: string) => {
  const removeToken = useRemoveToken();
  const createToken = useCreateToken(dialogId);
  const listToken = useListToken();
  const tokenList = useSelectTokenList();
  const creatingLoading = useOneNamespaceEffectsLoading('chatModel', [
    'createToken',
  ]);
  const listLoading = useOneNamespaceEffectsLoading('chatModel', ['list']);

  const showDeleteConfirm = useShowDeleteConfirm();

  const onRemoveToken = (token: string, tenantId: string) => {
    showDeleteConfirm({
      onOk: () => removeToken({ dialogId, tokens: [token], tenantId }),
    });
  };

  useEffect(() => {
    if (visible && dialogId) {
      listToken(dialogId);
    }
  }, [listToken, dialogId, visible]);

  return {
    removeToken: onRemoveToken,
    createToken,
    tokenList,
    creatingLoading,
    listLoading,
  };
};

type ChartStatsType = {
  [k in keyof IStats]: Array<{ xAxis: string; yAxis: number }>;
};

export const useSelectChartStatsList = (): ChartStatsType => {
  const stats: IStats = useSelectStats();
  // const stats = {
  //   pv: [
  //     ['2024-06-01', 1],
  //     ['2024-07-24', 3],
  //     ['2024-09-01', 10],
  //   ],
  //   uv: [
  //     ['2024-02-01', 0],
  //     ['2024-03-01', 99],
  //     ['2024-05-01', 3],
  //   ],
  //   speed: [
  //     ['2024-09-01', 2],
  //     ['2024-09-01', 3],
  //   ],
  //   tokens: [
  //     ['2024-09-01', 1],
  //     ['2024-09-01', 3],
  //   ],
  //   round: [
  //     ['2024-09-01', 0],
  //     ['2024-09-01', 3],
  //   ],
  //   thumb_up: [
  //     ['2024-09-01', 3],
  //     ['2024-09-01', 9],
  //   ],
  // };

  return Object.keys(stats).reduce((pre, cur) => {
    const item = stats[cur as keyof IStats];
    if (item.length > 0) {
      pre[cur as keyof IStats] = item.map((x) => ({
        xAxis: x[0] as string,
        yAxis: x[1] as number,
      }));
    }
    return pre;
  }, {} as ChartStatsType);
};

export const useShowTokenEmptyError = () => {
  const [messageApi, contextHolder] = message.useMessage();
  const { t } = useTranslate('chat');

  const showTokenEmptyError = useCallback(() => {
    messageApi.error(t('tokenError'));
  }, [messageApi, t]);
  return { showTokenEmptyError, contextHolder };
};

const getUrlWithToken = (token: string) => {
  const { protocol, host } = window.location;
  return `${protocol}//${host}/chat/share?shared_id=${token}`;
};

const useFetchTokenListBeforeOtherStep = (dialogId: string) => {
  const { showTokenEmptyError, contextHolder } = useShowTokenEmptyError();

  const listToken = useListToken();
  const tokenList = useSelectTokenList();

  const token =
    Array.isArray(tokenList) && tokenList.length > 0 ? tokenList[0].token : '';

  const handleOperate = useCallback(async () => {
    const data = await listToken(dialogId);
    const list = data.data;
    if (data.retcode === 0 && Array.isArray(list) && list.length > 0) {
      return list[0]?.token;
    } else {
      showTokenEmptyError();
      return false;
    }
  }, [dialogId, listToken, showTokenEmptyError]);

  return {
    token,
    contextHolder,
    handleOperate,
  };
};

export const useShowEmbedModal = (dialogId: string) => {
  const {
    visible: embedVisible,
    hideModal: hideEmbedModal,
    showModal: showEmbedModal,
  } = useSetModalState();

  const { handleOperate, token, contextHolder } =
    useFetchTokenListBeforeOtherStep(dialogId);

  const handleShowEmbedModal = useCallback(async () => {
    const succeed = await handleOperate();
    if (succeed) {
      showEmbedModal();
    }
  }, [handleOperate, showEmbedModal]);

  return {
    showEmbedModal: handleShowEmbedModal,
    hideEmbedModal,
    embedVisible,
    embedToken: token,
    errorContextHolder: contextHolder,
  };
};

export const usePreviewChat = (dialogId: string) => {
  const { handleOperate, contextHolder } =
    useFetchTokenListBeforeOtherStep(dialogId);

  const open = useCallback((t: string) => {
    window.open(getUrlWithToken(t), '_blank');
  }, []);

  const handlePreview = useCallback(async () => {
    const token = await handleOperate();
    if (token) {
      open(token);
    }
  }, [handleOperate, open]);

  return {
    handlePreview,
    contextHolder,
  };
};

//#endregion
