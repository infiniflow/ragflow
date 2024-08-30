import { MessageType } from '@/constants/chat';
import { useFetchFlow } from '@/hooks/flow-hooks';
import {
  useHandleMessageInputChange,
  useScrollToBottom,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { IAnswer, Message } from '@/interfaces/database/chat';
import { IMessage } from '@/pages/chat/interface';
import api from '@/utils/api';
import { buildMessageUuid } from '@/utils/chat';
import { message } from 'antd';
import trim from 'lodash/trim';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { receiveMessageError } from '../utils';

const antMessage = message;

export const useSelectCurrentMessages = () => {
  const { id: id } = useParams();
  const [currentMessages, setCurrentMessages] = useState<IMessage[]>([]);

  const { data: flowDetail, loading } = useFetchFlow();
  const messages = flowDetail.dsl.messages;
  const reference = flowDetail.dsl.reference;

  const ref = useScrollToBottom(currentMessages);

  const addNewestQuestion = useCallback(
    (message: Message, answer: string = '') => {
      setCurrentMessages((pre) => {
        return [
          ...pre,
          {
            ...message,
            id: buildMessageUuid(message),
          },
          {
            role: MessageType.Assistant,
            content: answer,
            id: buildMessageUuid({ ...message, role: MessageType.Assistant }),
          },
        ];
      });
    },
    [],
  );

  const addNewestAnswer = useCallback((answer: IAnswer) => {
    setCurrentMessages((pre) => {
      return [
        ...pre.slice(0, -1),
        {
          role: MessageType.Assistant,
          content: answer.answer,
          reference: answer.reference,
          id: buildMessageUuid({
            id: answer.id,
            role: MessageType.Assistant,
          }),
        },
      ];
    });
  }, []);

  const removeLatestMessage = useCallback(() => {
    setCurrentMessages((pre) => {
      const nextMessages = pre?.slice(0, -2) ?? [];
      return nextMessages;
      return [...pre, ...nextMessages];
    });
  }, []);

  useEffect(() => {
    if (id) {
      const nextMessages = messages.map((x) => ({ ...x, id: uuid() }));
      setCurrentMessages(nextMessages);
    }
  }, [messages, id]);

  return {
    currentMessages,
    reference,
    addNewestQuestion,
    removeLatestMessage,
    addNewestAnswer,
    ref,
    loading,
  };
};

export const useSelectNextMessages = () => {
  const { id: id } = useParams();
  const { data: flowDetail, loading } = useFetchFlow();
  const messages = flowDetail.dsl.messages;
  const reference = flowDetail.dsl.reference;
  const {
    derivedMessages,
    setDerivedMessages,
    ref,
    addNewestQuestion,
    addNewestAnswer,
    removeLatestMessage,
    removeMessageById,
    removeMessagesAfterCurrentMessage,
  } = useSelectDerivedMessages();

  useEffect(() => {
    if (id) {
      const nextMessages = messages.map((x) => ({ ...x, id: uuid() }));
      setDerivedMessages(nextMessages);
    }
  }, [messages, id, setDerivedMessages]);

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

export const useSendMessage = (
  addNewestQuestion: (message: Message, answer?: string) => void,
  removeLatestMessage: () => void,
  addNewestAnswer: (answer: IAnswer) => void,
) => {
  const { id: flowId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { refetch } = useFetchFlow();

  const { send, answer, done } = useSendMessageWithSse(api.runCanvas);

  const sendMessage = useCallback(
    async (message: Message) => {
      const params: Record<string, unknown> = {
        id: flowId,
      };
      if (message.content) {
        params.message = message.content;
        params.message_id = message.id;
      }
      const res = await send(params);

      if (receiveMessageError(res)) {
        antMessage.error(res?.data?.retmsg);

        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      } else {
        refetch(); // pull the message list after sending the message successfully
      }
    },
    [flowId, removeLatestMessage, setValue, send, refetch],
  );

  const handleSendMessage = useCallback(
    async (message: Message) => {
      sendMessage(message);
    },
    [sendMessage],
  );

  useEffect(() => {
    if (answer.answer) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer]);

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

  return {
    handlePressEnter,
    handleInputChange,
    value,
    loading: !done,
  };
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
  const { id: flowId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { refetch } = useFetchFlow();

  const { send, answer, done } = useSendMessageWithSse(api.runCanvas);

  const sendMessage = useCallback(
    async ({ message }: { message: Message; messages?: Message[] }) => {
      const params: Record<string, unknown> = {
        id: flowId,
      };
      if (message.content) {
        params.message = message.content;
        params.message_id = message.id;
      }
      const res = await send(params);

      if (receiveMessageError(res)) {
        antMessage.error(res?.data?.retmsg);

        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      } else {
        refetch(); // pull the message list after sending the message successfully
      }
    },
    [flowId, removeLatestMessage, setValue, send, refetch],
  );

  const handleSendMessage = useCallback(
    async (message: Message) => {
      sendMessage({ message });
    },
    [sendMessage],
  );

  useEffect(() => {
    if (answer.answer) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer]);

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
  };
};
