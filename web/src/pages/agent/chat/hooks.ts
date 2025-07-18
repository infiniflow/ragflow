import sonnerMessage from '@/components/ui/message';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
} from '@/hooks/logic-hooks';
import { useFetchAgent } from '@/hooks/use-agent-request';
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
import { useGetBeginNodeDataQuery } from '../hooks/use-get-begin-query';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { receiveMessageError } from '../utils';

export const useSelectNextMessages = () => {
  const { data: flowDetail, loading } = useFetchAgent();
  const reference = flowDetail.dsl.retrieval;
  const {
    derivedMessages,
    ref,
    addNewestQuestion,
    addNewestAnswer,
    removeLatestMessage,
    removeMessageById,
    removeMessagesAfterCurrentMessage,
    addNewestOneQuestion,
    addNewestOneAnswer,
  } = useSelectDerivedMessages();

  return {
    reference,
    loading,
    derivedMessages,
    ref,
    addNewestQuestion,
    addNewestAnswer,
    removeLatestMessage,
    removeMessageById,
    addNewestOneQuestion,
    addNewestOneAnswer,
    removeMessagesAfterCurrentMessage,
  };
};

function findMessageFromList(eventList: IEventList) {
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

function findInputFromList(eventList: IEventList) {
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

const useGetBeginNodePrologue = () => {
  const getNode = useGraphStore((state) => state.getNode);

  return useMemo(() => {
    const formData = get(getNode(BeginId), 'data.form', {});
    if (formData?.enablePrologue) {
      return formData?.prologue;
    }
  }, [getNode]);
};

export const useSendNextMessage = () => {
  const {
    reference,
    loading,
    derivedMessages,
    ref,
    removeLatestMessage,
    removeMessageById,
    addNewestOneQuestion,
    addNewestOneAnswer,
  } = useSelectNextMessages();
  const { id: agentId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { refetch } = useFetchAgent();
  const { addEventList } = useContext(AgentChatLogContext);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const [messageEndEventList, setMessageEndEventList] = useState<
    IMessageEndEvent[]
  >([]);

  const { send, answerList, done, stopOutputMessage } = useSendMessageBySSE(
    api.runCanvas,
  );

  const prologue = useGetBeginNodePrologue();

  const sendMessage = useCallback(
    async ({ message }: { message: Message; messages?: Message[] }) => {
      const params: Record<string, unknown> = {
        id: agentId,
      };

      params.running_hint_text = i18n.t('flow.runningHintText', {
        defaultValue: 'is running...🕞',
      });
      if (message.content) {
        const query = getBeginNodeDataQuery();

        params.query = message.content;
        // params.message_id = message.id;
        params.inputs = transferInputsArrayToObject(query); // begin operator inputs
      }
      const res = await send(params);

      if (receiveMessageError(res)) {
        sonnerMessage.error(res?.data?.message);

        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      } else {
        refetch(); // pull the message list after sending the message successfully
      }
    },
    [
      agentId,
      send,
      getBeginNodeDataQuery,
      setValue,
      removeLatestMessage,
      refetch,
    ],
  );

  const handleSendMessage = useCallback(
    async (message: Message) => {
      sendMessage({ message });
    },
    [sendMessage],
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
  }, [addEventList.length, answerList]);

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

  const handlePressEnter = useCallback(() => {
    if (trim(value) === '') return;
    const id = uuid();
    if (done) {
      setValue('');
      handleSendMessage({ id, content: value.trim(), role: MessageType.User });
    }
    addNewestOneQuestion({
      content: value,
      id,
      role: MessageType.User,
    });
  }, [value, done, addNewestOneQuestion, setValue, handleSendMessage]);

  const sendFormMessage = useCallback(
    (body: { id?: string; inputs: Record<string, BeginQuery> }) => {
      send(body);
      addNewestOneQuestion({
        content: Object.entries(body.inputs)
          .map(([key, val]) => `${key}: ${val.value}`)
          .join('<br/>'),
        role: MessageType.User,
      });
    },
    [addNewestOneQuestion, send],
  );

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
    if (prologue) {
      addNewestOneAnswer({
        answer: prologue,
      });
    }
  }, [
    addNewestOneAnswer,
    agentId,
    getBeginNodeDataQuery,
    prologue,
    send,
    sendFormMessage,
  ]);

  useEffect(() => {
    addEventList(answerList);
  }, [addEventList, answerList]);

  return {
    handlePressEnter,
    handleInputChange,
    value,
    sendLoading: !done,
    reference,
    loading,
    derivedMessages,
    ref,
    removeMessageById,
    stopOutputMessage,
    send,
    sendFormMessage,
    findReferenceByMessageId,
  };
};
