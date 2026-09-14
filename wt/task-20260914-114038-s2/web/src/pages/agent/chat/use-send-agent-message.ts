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
import { Message } from '@/interfaces/database/chat';
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
import { shouldSplitMessage } from '../utils/chat';

export function findMessageFromList(eventList: IEventList) {
  const messageEventList = eventList.filter(
    (x) => x.event === MessageEventType.Message,
  ) as IMessageEvent[];

  let nextContent = '';

  let startIndex = -1;
  let endIndex = -1;
  let audioBinary = undefined;
  messageEventList.forEach((x, idx) => {
    const { data } = x;
    const { content, start_to_think, end_to_think, audio_binary } = data;
    if (audio_binary) {
      audioBinary = audio_binary;
    }
    if (start_to_think === true) {
      nextContent += '<think>' + content;
      startIndex = idx;
      return;
    }

    if (end_to_think === true) {
      endIndex = idx;
      nextContent += content + '</think>';
      return;
    }

    nextContent += content;
  });

  const currentIdx = messageEventList.length - 1;

  // Make sure that after start_to_think === true and before end_to_think === true, add a </think> tag at the end.
  if (startIndex >= 0 && startIndex <= currentIdx && endIndex === -1) {
    nextContent += '</think>';
  }

  const workflowFinished = eventList.find(
    (x) => x.event === MessageEventType.WorkflowFinished,
  ) as IMessageEvent;
  const messageEndEvent = [...eventList]
    .reverse()
    .find((x) => x.event === MessageEventType.MessageEnd) as IMessageEndEvent;
  return {
    id: eventList[0]?.message_id,
    content: nextContent,
    audio_binary: audioBinary,
    attachment:
      workflowFinished?.data?.outputs?.attachment ||
      messageEndEvent?.data?.attachment ||
      {},
    downloads:
      workflowFinished?.data?.outputs?.downloads ||
      messageEndEvent?.data?.downloads ||
      [],
  };
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
      const event = messageEndEventList.find(
        (item) => item.message_id === messageId,
      );
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
          nextList.every((x) => x.message_id !== messageEndEvent.message_id)
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
  activeSessionId,
}: {
  url?: string;
  addEventList?: (data: IEventList, messageId: string) => void;
  beginParams?: BeginQuery[];
  isShared?: boolean;
  refetch?: () => void;
  isTaskMode?: boolean;
  releaseMode?: string | null;
  /**
   * Session the page is currently displaying. When provided, streamed
   * frames that belong to another session are not written into the
   * displayed message list (the user may switch sessions in Explore
   * while an answer is still streaming).
   */
  activeSessionId?: string;
}) => {
  const { id: agentId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const inputs = useSelectBeginNodeDataInputs();
  const [sessionId, setSessionId] = useState<string | null>(null);
  const { send, answerList, done, stopOutputMessage, resetAnswerList } =
    useSendMessageBySSE(url || api.agentChatCompletion);
  const firstAnswer = answerList[0];
  // Session that owns the in-flight stream; every SSE frame carries the
  // session_id it belongs to.
  const streamSessionId = firstAnswer?.session_id;
  // Session the pending request was sent to. It is known before the first
  // SSE frame arrives, so the stream can already be attributed to its
  // session during connection setup.
  const [requestedSessionId, setRequestedSessionId] = useState<
    string | null | undefined
  >();
  // Bumped when derivedMessages is replaced externally (Explore hydrates
  // persisted messages when a session is re-selected) while a stream
  // owned by the displayed session is in flight, so the streamed answer
  // is re-applied on top of the hydrated history — the effect below
  // otherwise only re-runs when a new frame arrives.
  const [streamReplayToken, setStreamReplayToken] = useState(0);
  const reapplyStreamedAnswer = useCallback(
    () => setStreamReplayToken((token) => token + 1),
    [],
  );
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
        // Remember the owner before the first frame arrives so
        // connection-setup loading states are attributed to the right
        // session.
        setRequestedSessionId((exploreSessionId || sessionId) ?? null);
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
      setRequestedSessionId(sessionId ?? null);
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
      setTimeout(() => {
        scrollToBottom();
      }, 100);
    },
    [
      value,
      done,
      addNewestOneQuestion,
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
    // The stream belongs to the session it was started in. If the user has
    // switched to a different session while the answer is streaming, do not
    // write the incoming frames into the message list being displayed.
    if (
      activeSessionId !== undefined &&
      streamSessionId !== undefined &&
      streamSessionId !== activeSessionId
    ) {
      return;
    }
    const { content, id, attachment, audio_binary, downloads } =
      findMessageFromList(answerList);
    const inputAnswer = findInputFromList(answerList);
    const answer = content || getLatestError(answerList);

    if (answerList.length > 0) {
      const shouldSplit = shouldSplitMessage(answerList, content);

      if (shouldSplit) {
        addNewestOneAnswer({
          answer: answer ?? '',
          audio_binary: audio_binary,
          attachment: attachment as IAttachment,
          downloads,
          id,
        });
        addNewestOneAnswer({
          answer: '',
          ...inputAnswer,
          id: `${id}${MessageWaitSuffix}`,
        });
      } else {
        addNewestOneAnswer({
          answer: answer ?? '',
          audio_binary: audio_binary,
          attachment: attachment as IAttachment,
          downloads,
          id,
          ...inputAnswer,
        });
      }
    }
  }, [
    activeSessionId,
    answerList,
    addNewestOneAnswer,
    streamSessionId,
    streamReplayToken,
  ]);

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
    streamSessionId,
    requestedSessionId,
    reapplyStreamedAnswer,
  };
};
