import sonnerMessage from '@/components/ui/message';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
} from '@/hooks/logic-hooks';
import {
  IAttachment,
  IEventList,
  IMessageEndData,
  IMessageEndEvent,
  IMessageEvent,
  MessageEventType,
  useSendMessageBySSE,
} from '@/hooks/use-send-message';
import { IDocumentDownloadInfo, Message } from '@/interfaces/database/chat';
import i18n from '@/locales/config';
import api from '@/utils/api';
import { get } from 'lodash';
import trim from 'lodash/trim';
import {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useParams, useSearchParams } from 'react-router';
import { v4 as uuid } from 'uuid';
import { BeginId } from '../constant';
import { MessageWaitSuffix } from '../constant/chat';
import { AgentChatLogContext } from '../context';
import { transferInputsArrayToObject } from '../form/begin-form/use-watch-change';
import {
  useIsTaskMode,
  useSelectBeginNodeDataInputs,
} from '../hooks/use-get-begin-query';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { receiveMessageError } from '../utils';

export interface IMessageSegment {
  id: string;
  content: string;
  audio_binary?: string;
  attachment?: IAttachment;
  downloads: IDocumentDownloadInfo[];
}

/**
 * Разбивает SSE-поток одного прогона агента на отдельные области ответа.
 *
 * Каждая область ответа — это последовательность `message`-событий,
 * завершённая своим `message_end`. Так получается и при нескольких
 * Message-компонентах по ходу работы агента, и при одном Message-компоненте
 * с включённым `emit_all`. Возвращается по одному элементу на область.
 */
export function findMessageSegmentsFromList(eventList: IEventList): IMessageSegment[] {
  const segments: IMessageSegment[] = [];
  const messageId = eventList[0]?.message_id ?? '';
  let currentContent = '';
  let thinking = false;
  let audioBinary: string | undefined;

  const finalizeSegment = (
    attachment?: IAttachment,
    downloads: IDocumentDownloadInfo[] = [],
  ) => {
    if (thinking) {
      currentContent += '</think>';
      thinking = false;
    }
    segments.push({
      id: segments.length === 0 ? messageId : `${messageId}#${segments.length}`,
      content: currentContent,
      audio_binary: audioBinary,
      attachment,
      downloads: dedupeDownloads(downloads),
    });
    currentContent = '';
    audioBinary = undefined;
  };

  for (const x of eventList) {
    if (x.event === MessageEventType.Message && x.data) {
      const { content = '', start_to_think, end_to_think, audio_binary } = x.data;
      if (audio_binary) {
        audioBinary = audio_binary;
      }
      if (start_to_think === true) {
        currentContent += '<think>' + content;
        thinking = true;
      } else if (end_to_think === true) {
        currentContent += content + '</think>';
        thinking = false;
      } else {
        currentContent += content;
      }
    } else if (x.event === MessageEventType.MessageEnd && x.data) {
      finalizeSegment(x.data.attachment, x.data.downloads);
    }
  }

  const workflowFinished = eventList.find(
    (x) => x.event === MessageEventType.WorkflowFinished,
  ) as IMessageEvent | undefined;
  const trailingAttachment = workflowFinished?.data?.outputs?.attachment;
  const trailingDownloads = (workflowFinished?.data?.outputs?.downloads ?? []) as
    | IDocumentDownloadInfo[]
    | undefined;

  if (currentContent !== '' || segments.length === 0) {
    finalizeSegment(trailingAttachment, trailingDownloads ?? []);
  } else if (trailingDownloads?.length) {
    const last = segments[segments.length - 1];
    last.downloads = dedupeDownloads([...last.downloads, ...trailingDownloads]);
  }

  return segments;
}

function dedupeDownloads(downloads: IDocumentDownloadInfo[]) {
  const seen = new Set<string>();
  return downloads.filter((d) => {
    const key = d?.doc_id || d?.filename;
    if (!key || seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

export function findInputFromList(eventList: IEventList) {
  const inputEvent = eventList.find(
    (x) => x.event === MessageEventType.UserInputs,
  );

  if (!inputEvent) {
    return {};
  }

  return {
    id: inputEvent?.message_id,
    data: inputEvent?.data,
  };
}

export function getLatestError(eventList: IEventList) {
  const latest = eventList.at(-1) as
    | { code?: number; message?: string }
    | undefined;
  return (
    get(latest, 'data.outputs._ERROR') ||
    (latest?.code && latest.code !== 0 ? latest?.message : undefined)
  );
}

export const useGetBeginNodePrologue = () => {
  const getNode = useGraphStore((state) => state.getNode);
  const formData = get(getNode(BeginId), 'data.form', {});

  return useMemo(() => {
    if (formData?.enablePrologue) {
      return formData?.prologue;
    }
  }, [formData?.enablePrologue, formData?.prologue]);
};

export function useFindMessageReference(answerList: IEventList) {
  const [messageEndEventList, setMessageEndEventList] = useState<
    IMessageEndEvent[]
  >([]);

  const findReferenceByMessageId = useCallback(
    (messageId: string) => {
      const [, suffix] = String(messageId ?? '').split('#');
      const index = suffix !== undefined ? Number(suffix) : 0;
      const event = messageEndEventList[index];
      if (event) {
        return (event?.data as IMessageEndData)?.reference;
      }
    },
    [messageEndEventList],
  );

  useEffect(() => {
    const messageEndEvent = answerList.find(
      (x) => x.event === MessageEventType.MessageEnd,
    );
    if (messageEndEvent) {
      setMessageEndEventList((list) => {
        const nextList = [...list];
        if (
          nextList.every((x) => x !== messageEndEvent)
        ) {
          nextList.push(messageEndEvent as IMessageEndEvent);
        }
        return nextList;
      });
    }
  }, [answerList]);

  return { findReferenceByMessageId };
}

interface UploadResponseDataType {
  created_at: number;
  created_by: string;
  extension: string;
  id: string;
  mime_type: string;
  name: string;
  preview_url: null;
  size: number;
}

export function useSetUploadResponseData() {
  const [uploadResponseList, setUploadResponseList] = useState<
    UploadResponseDataType[]
  >([]);
  const [fileList, setFileList] = useState<File[]>([]);

  const append = useCallback((data: UploadResponseDataType, files: File[]) => {
    setUploadResponseList((prev) => [...prev, data]);
    setFileList((pre) => [...pre, ...files]);
  }, []);

  const clear = useCallback(() => {
    setUploadResponseList([]);
    setFileList([]);
  }, []);

  const removeFile = useCallback((file: File) => {
    setFileList((prev) => prev.filter((f) => f !== file));
    setUploadResponseList((prev) =>
      prev.filter((item) => item.name !== file.name),
    );
  }, []);

  return {
    uploadResponseList,
    fileList,
    setUploadResponseList,
    appendUploadResponseList: append,
    clearUploadResponseList: clear,
    removeFile,
  };
}

export const buildRequestBody = (value: string = '') => {
  const id = uuid();
  const msgBody = {
    id,
    content: value.trim(),
    role: MessageType.User,
  };

  return msgBody;
};

export const useSendAgentMessage = ({
  url,
  addEventList,
  beginParams,
  isShared,
  refetch,
  isTaskMode: isTask,
  releaseMode,
}: {
  url?: string;
  addEventList?: (data: IEventList, messageId: string) => void;
  beginParams?: BeginQuery[];
  isShared?: boolean;
  refetch?: () => void;
  isTaskMode?: boolean;
  releaseMode?: string | null;
}) => {
  const { id: agentId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const inputs = useSelectBeginNodeDataInputs();
  const [sessionId, setSessionId] = useState<string | null>(null);
  const { send, answerList, done, stopOutputMessage, resetAnswerList } =
    useSendMessageBySSE(url || api.agentChatCompletion);
  const firstAnswer = answerList[0];
  const messageId = useMemo(() => {
    return firstAnswer?.message_id;
  }, [firstAnswer]);

  const isTaskMode = useIsTaskMode(isTask);

  const { findReferenceByMessageId } = useFindMessageReference(answerList);
  const prologue = useGetBeginNodePrologue();
  const {
    derivedMessages,
    scrollRef,
    messageContainerRef,
    removeLatestMessage,
    removeMessageById,
    addNewestOneQuestion,
    addNewestOneAnswer,
    removeAllMessages,
    removeAllMessagesExceptFirst,
    scrollToBottom,
    addPrologue,
    setDerivedMessages,
  } = useSelectDerivedMessages();
  const { addEventList: addEventListFun } = useContext(AgentChatLogContext);
  const {
    appendUploadResponseList,
    clearUploadResponseList,
    uploadResponseList,
    fileList,
    removeFile,
  } = useSetUploadResponseData();

  const [searchParams] = useSearchParams();

  const userId = searchParams.get('userId');

  // Временный id пустого placeholder, создаваемого сразу при отправке вопроса,
  // чтобы крутилка появлялась мгновенно — до первого SSE-события. Когда приходит
  // первый message_id, placeholder перепривязывается к нему (см. useEffect ниже).
  const pendingAnswerIdRef = useRef<string | null>(null);

  const stopConversation = useCallback(() => {
    stopOutputMessage();
  }, [stopOutputMessage]);

  const sendMessage = useCallback(
    async ({
      message,
      beginInputs,
      exploreSessionId,
    }: {
      message: Message;
      messages?: Message[];
      beginInputs?: BeginQuery[];
      exploreSessionId?: string;
    }) => {
      const params: Record<string, unknown> = {
        agent_id: agentId,
        stream: true,
      };

      params.running_hint_text = i18n.t('flow.runningHintText', {
        defaultValue: 'is running...🕞',
      });
      params['openai-compatible'] = false;
      if (typeof message.content === 'string') {
        const query = inputs;

        params.query = message.content;
        // params.message_id = message.id;
        params.inputs = transferInputsArrayToObject(
          beginInputs || beginParams || query,
        ); // begin operator inputs

        params.files = uploadResponseList;

        // Prefer the session selected by the outer page state.
        // The hook keeps its own session cache for streamed replies, but that cache
        // can lag behind when the user switches sessions in Explore.
        params.session_id = exploreSessionId || sessionId;
        if (releaseMode) {
          params.release = releaseMode;
        }

        if (userId) {
          params.user_id = userId;
        }
      }

      try {
        const res = await send(params);

        clearUploadResponseList();

        if (receiveMessageError(res)) {
          sonnerMessage.error(res?.data?.message);

          // cancel loading
          setValue(message.content);
          removeLatestMessage();
        } else {
          refetch?.(); // pull the message list after sending the message successfully
        }
      } catch (error) {
        console.log('🚀 ~ useSendAgentMessage ~ error:', error);
      }
    },
    [
      agentId,
      inputs,
      beginParams,
      uploadResponseList,
      sessionId,
      releaseMode,
      userId,
      send,
      clearUploadResponseList,
      setValue,
      removeLatestMessage,
      refetch,
    ],
  );

  const sendFormMessage = useCallback(
    async (body: { inputs: Record<string, BeginQuery> }) => {
      addNewestOneQuestion({
        content: Object.entries(body.inputs)
          .map(([, val]) => `${val.name}: ${val.value}`)
          .join('<br/>'),
        role: MessageType.User,
      });
      await send({
        ...body,
        ...(isShared ? {} : { agent_id: agentId }),
        stream: true,
        session_id: sessionId,
        ...(releaseMode ? { release: releaseMode } : {}),
      });
      refetch?.();
    },
    [
      addNewestOneQuestion,
      agentId,
      isShared,
      refetch,
      releaseMode,
      send,
      sessionId,
    ],
  );

  // reset session
  const resetSession = useCallback(() => {
    stopConversation();
    resetAnswerList();
    pendingAnswerIdRef.current = null;
    setSessionId(null);
    if (isTaskMode) {
      removeAllMessages();
    } else {
      removeAllMessagesExceptFirst();
    }
  }, [
    stopConversation,
    resetAnswerList,
    isTaskMode,
    removeAllMessages,
    removeAllMessagesExceptFirst,
  ]);

  const handlePressEnter = useCallback(
    ({ exploreSessionId }: { exploreSessionId?: string } = {}) => {
      if (trim(value) === '' || !done) return;
      const msgBody = buildRequestBody(value);
      if (done) {
        setValue('');
        sendMessage({
          message: msgBody,
          exploreSessionId,
        });
      }
      addNewestOneQuestion({ ...msgBody, files: fileList });
      // Мгновенный placeholder «три точки»: сообщение ассистента появляется
      // сразу, не дожидаясь первого SSE-события. После прихода первого
      // message_id оно будет перепривязано к реальному id.
      const pendingId = `${msgBody.id}-pending`;
      pendingAnswerIdRef.current = pendingId;
      addNewestOneAnswer({ answer: '', id: pendingId });
      setTimeout(() => {
        scrollToBottom();
      }, 100);
    },
    [
      value,
      done,
      addNewestOneQuestion,
      addNewestOneAnswer,
      fileList,
      setValue,
      sendMessage,
      scrollToBottom,
    ],
  );

  const sendedTaskMessage = useRef(false);

  const sendMessageInTaskMode = useCallback(() => {
    if (isShared || !isTaskMode || sendedTaskMessage.current) {
      return;
    }
    const msgBody = buildRequestBody('');

    sendMessage({
      message: msgBody,
    });
    sendedTaskMessage.current = true;
  }, [isShared, isTaskMode, sendMessage]);

  useEffect(() => {
    sendMessageInTaskMode();
  }, [sendMessageInTaskMode]);

  useEffect(() => {
    if (answerList.length === 0) return;
    const segments = findMessageSegmentsFromList(answerList);
    const inputAnswer = findInputFromList(answerList);
    const error = getLatestError(answerList);
    const messageId = answerList[0]?.message_id;

    // Мгновенный placeholder из handlePressEnter имеет временный id
    // («<question>-pending»). Как только приходит первый message_id — меняем
    // ему id на реальный, чтобы addNewestOneAnswer ниже обновил его, а не
    // создал дубликат.
    const pendingId = pendingAnswerIdRef.current;
    if (pendingId && messageId) {
      pendingAnswerIdRef.current = null;
      setDerivedMessages((pre) =>
        pre.map((x) => (x.id === pendingId ? { ...x, id: messageId } : x)),
      );
    }

    if (segments.length === 0) {
      if (error) {
        addNewestOneAnswer({
          answer: error,
          id: messageId,
        });
      } else if (
        messageId &&
        !answerList.some(
          (x) => x.event === MessageEventType.WorkflowFinished,
        )
      ) {
        // Пока агент работает (первые события уже пришли, но ответ ещё не
        // завершён message_end), держим пустой placeholder — «три точки».
        // Его id совпадает с id первого сегмента, поэтому по завершении
        // области addNewestOneAnswer обновит сообщение вместо дубликата.
        addNewestOneAnswer({
          answer: '',
          id: messageId,
        });
      }
      return;
    }

    segments.forEach((segment, index) => {
      const hasContent = segment.content !== '';
      const hasDownloads = (segment.downloads ?? []).length > 0;
      const hasAttachment = Boolean(segment.attachment?.doc_id);
      if (!hasContent && !hasDownloads && !hasAttachment) {
        return;
      }
      addNewestOneAnswer({
        answer: segment.content || (index === 0 ? error ?? '' : ''),
        audio_binary: segment.audio_binary,
        attachment: (segment.attachment ?? {}) as IAttachment,
        downloads: segment.downloads,
        id: segment.id,
      });
    });

    if (inputAnswer.id && segments.length > 0) {
      const lastId = segments[segments.length - 1].id;
      addNewestOneAnswer({
        answer: '',
        ...inputAnswer,
        id: `${lastId}${MessageWaitSuffix}`,
      });
    }
  }, [answerList, addNewestOneAnswer, setDerivedMessages]);

  useEffect(() => {
    if (isTaskMode) {
      return;
    }
    if (prologue) {
      addPrologue(prologue);
    }
  }, [
    addNewestOneAnswer,
    addPrologue,
    agentId,
    isTaskMode,
    prologue,
    send,
    sendFormMessage,
  ]);

  useEffect(() => {
    if (typeof addEventList === 'function') {
      addEventList(answerList, messageId);
    } else if (typeof addEventListFun === 'function') {
      addEventListFun(answerList, messageId);
    }
  }, [addEventList, answerList, addEventListFun, messageId]);

  useEffect(() => {
    if (firstAnswer?.session_id) {
      setSessionId(firstAnswer.session_id);
    }
  }, [firstAnswer]);

  return {
    value,
    sendLoading: !done,
    derivedMessages,
    scrollRef,
    messageContainerRef,
    handlePressEnter,
    handleInputChange,
    removeMessageById,
    stopOutputMessage: stopConversation,
    send,
    sendFormMessage,
    resetSession,
    findReferenceByMessageId,
    appendUploadResponseList,
    addNewestOneAnswer,
    sendMessage,
    removeFile,
    setDerivedMessages,
    addPrologue,
  };
};
