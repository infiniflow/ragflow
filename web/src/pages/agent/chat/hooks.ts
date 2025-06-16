import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
} from '@/hooks/logic-hooks';
import { useFetchAgent } from '@/hooks/use-agent-request';
import {
  IEventList,
  IMessageEvent,
  MessageEventType,
  useSendMessageBySSE,
} from '@/hooks/use-send-message';
import { Message } from '@/interfaces/database/chat';
import i18n from '@/locales/config';
import api from '@/utils/api';
import { message } from 'antd';
import { get } from 'lodash';
import trim from 'lodash/trim';
import { useCallback, useContext, useEffect, useMemo } from 'react';
import { useParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { BeginId } from '../constant';
import { AgentChatLogContext } from '../context';
import useGraphStore from '../store';
import { receiveMessageError } from '../utils';

const antMessage = message;

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
    removeMessagesAfterCurrentMessage,
  };
};

function findMessageFromList(eventList: IEventList) {
  const messageEventList = eventList.filter(
    (x) => x.event === MessageEventType.Message,
  ) as IMessageEvent[];
  return {
    id: messageEventList[0]?.message_id,
    content: messageEventList.map((x) => x.data.content).join(''),
  };
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
    addNewestQuestion,
    addNewestAnswer,
    removeLatestMessage,
    removeMessageById,
  } = useSelectNextMessages();
  const { id: agentId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { refetch } = useFetchAgent();
  const { addEventList } = useContext(AgentChatLogContext);

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
        antMessage.error(res?.data?.message);

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
    if (content) {
      addNewestAnswer({
        answer: content,
        id: id,
      });
    }
  }, [answerList, addNewestAnswer]);

  const handlePressEnter = useCallback(() => {
    if (trim(value) === '') return;
    const id = uuid();
    if (done) {
      setValue('');
      handleSendMessage({ id, content: value.trim(), role: MessageType.User });
    }
    addNewestQuestion({
      content: value,
      id,
      role: MessageType.User,
    });
  }, [addNewestQuestion, handleSendMessage, done, setValue, value]);

  useEffect(() => {
    if (prologue) {
      addNewestAnswer({
        answer: prologue,
      });
    }
  }, [addNewestAnswer, prologue]);

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
  };
};
