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
  IMessageEvent,
  MessageEventType,
  useSendMessageBySSE,
} from '@/hooks/use-send-message';
import { Message } from '@/interfaces/database/chat';
import i18n from '@/locales/config';
import api from '@/utils/api';
import { get } from 'lodash';
import trim from 'lodash/trim';
import { useCallback, useContext, useEffect, useMemo } from 'react';
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
  return {
    id: eventList[0]?.message_id,
    content: messageEventList.map((x) => x.data.content).join(''),
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
        params.query = message.content;
        // params.message_id = message.id;
        params.inputs = {}; // begin operator inputs
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
    [agentId, send, setValue, removeLatestMessage, refetch],
  );

  const handleSendMessage = useCallback(
    async (message: Message) => {
      sendMessage({ message });
    },
    [sendMessage],
  );

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

  useEffect(() => {
    const query = getBeginNodeDataQuery();
    if (query.length > 0) {
      send({ id: agentId, inputs: transferInputsArrayToObject(query) });
    } else if (prologue) {
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
  };
};
