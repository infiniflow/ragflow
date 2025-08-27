import sonnerMessage from '@/components/ui/message';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
} from '@/hooks/logic-hooks';
import {
  IEventList,
  IInputEvent,
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
import { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { BeginId } from '../constant';
import { AgentChatLogContext } from '../context';
import { transferInputsArrayToObject } from '../form/begin-form/use-watch-change';
import { useSelectBeginNodeDataInputs } from '../hooks/use-get-begin-query';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { receiveMessageError } from '../utils';

export function findMessageFromList(eventList: IEventList) {
  const messageEventList = eventList.filter(
    (x) => x.event === MessageEventType.Message,
  ) as IMessageEvent[];

  let nextContent = '';

  let startIndex = -1;
  let endIndex = -1;

  messageEventList.forEach((x, idx) => {
    const { data } = x;
    const { content, start_to_think, end_to_think } = data;
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

  return {
    id: eventList[0]?.message_id,
    content: nextContent,
  };
}

export function findInputFromList(eventList: IEventList) {
  const inputEvent = eventList.find(
    (x) => x.event === MessageEventType.UserInputs,
  ) as IInputEvent;

  if (!inputEvent) {
    return {};
  }

  return {
    id: inputEvent?.message_id,
    data: inputEvent?.data,
  };
}

export function getLatestError(eventList: IEventList) {
  return get(eventList.at(-1), 'data.outputs._ERROR');
}

export const useGetBeginNodePrologue = () => {
  const getNode = useGraphStore((state) => state.getNode);

  return useMemo(() => {
    const formData = get(getNode(BeginId), 'data.form', {});
    if (formData?.enablePrologue) {
      return formData?.prologue;
    }
  }, [getNode]);
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

  return {
    uploadResponseList,
    fileList,
    setUploadResponseList,
    appendUploadResponseList: append,
    clearUploadResponseList: clear,
  };
}

export const useSendAgentMessage = (
  url?: string,
  addEventList?: (data: IEventList, messageId: string) => void,
  beginParams?: any[],
) => {
  const { id: agentId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const inputs = useSelectBeginNodeDataInputs();
  const [sessionId, setSessionId] = useState<string | null>(null);
  const { send, answerList, done, stopOutputMessage, resetAnswerList } =
    useSendMessageBySSE(url || api.runCanvas);
  const messageId = useMemo(() => {
    return answerList[0]?.message_id;
  }, [answerList]);

  // const { refetch } = useFetchAgent();

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
    scrollToBottom,
  } = useSelectDerivedMessages();
  const { addEventList: addEventListFun } = useContext(AgentChatLogContext);
  const {
    appendUploadResponseList,
    clearUploadResponseList,
    uploadResponseList,
    fileList,
  } = useSetUploadResponseData();

  const sendMessage = useCallback(
    async ({ message }: { message: Message; messages?: Message[] }) => {
      const params: Record<string, unknown> = {
        id: agentId,
      };

      params.running_hint_text = i18n.t('flow.runningHintText', {
        defaultValue: 'is running...🕞',
      });
      if (message.content) {
        const query = inputs;

        params.query = message.content;
        // params.message_id = message.id;
        params.inputs = transferInputsArrayToObject(
          beginParams ? beginParams : query,
        ); // begin operator inputs

        params.files = uploadResponseList;

        params.session_id = sessionId;
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
          // refetch(); // pull the message list after sending the message successfully
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
      send,
      clearUploadResponseList,
      setValue,
      removeLatestMessage,
    ],
  );

  const sendFormMessage = useCallback(
    async (body: { id?: string; inputs: Record<string, BeginQuery> }) => {
      addNewestOneQuestion({
        content: Object.entries(body.inputs)
          .map(([key, val]) => `${key}: ${val.value}`)
          .join('<br/>'),
        role: MessageType.User,
      });
      await send({ ...body, session_id: sessionId });
      // refetch();
    },
    [addNewestOneQuestion, send, sessionId],
  );

  // reset session
  const resetSession = useCallback(() => {
    stopOutputMessage();
    resetAnswerList();
    setSessionId(null);
    removeAllMessages();
  }, [resetAnswerList, removeAllMessages, stopOutputMessage]);

  const handlePressEnter = useCallback(() => {
    if (trim(value) === '') return;
    const id = uuid();
    const msgBody = {
      id,
      content: value.trim(),
      role: MessageType.User,
    };
    if (done) {
      setValue('');
      sendMessage({
        message: msgBody,
      });
    }
    addNewestOneQuestion({ ...msgBody, files: fileList });
    setTimeout(() => {
      scrollToBottom();
    }, 100);
  }, [
    value,
    done,
    addNewestOneQuestion,
    fileList,
    setValue,
    sendMessage,
    scrollToBottom,
  ]);

  useEffect(() => {
    const { content, id } = findMessageFromList(answerList);
    const inputAnswer = findInputFromList(answerList);
    if (answerList.length > 0) {
      addNewestOneAnswer({
        answer: content || getLatestError(answerList),
        id: id,
        ...inputAnswer,
      });
    }
  }, [answerList, addNewestOneAnswer]);

  useEffect(() => {
    if (prologue) {
      addNewestOneAnswer({
        answer: prologue,
      });
    }
  }, [addNewestOneAnswer, agentId, prologue, send, sendFormMessage]);

  useEffect(() => {
    if (typeof addEventList === 'function') {
      addEventList(answerList, messageId);
    } else if (typeof addEventListFun === 'function') {
      addEventListFun(answerList, messageId);
    }
  }, [addEventList, answerList, addEventListFun, messageId]);

  useEffect(() => {
    if (answerList[0]?.session_id) {
      setSessionId(answerList[0]?.session_id);
    }
  }, [answerList]);

  return {
    value,
    sendLoading: !done,
    derivedMessages,
    scrollRef,
    messageContainerRef,
    handlePressEnter,
    handleInputChange,
    removeMessageById,
    stopOutputMessage,
    send,
    sendFormMessage,
    resetSession,
    findReferenceByMessageId,
    appendUploadResponseList,
    addNewestOneAnswer,
  };
};
