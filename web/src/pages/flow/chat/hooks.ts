import { MessageType } from '@/constants/chat';
import { useFetchFlow } from '@/hooks/flow-hooks';
import {
  useHandleMessageInputChange,
  useScrollToBottom,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { IAnswer } from '@/interfaces/database/chat';
import { IMessage } from '@/pages/chat/interface';
import api from '@/utils/api';
import { message } from 'antd';
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
    (message: string, answer: string = '') => {
      setCurrentMessages((pre) => {
        return [
          ...pre,
          {
            role: MessageType.User,
            content: message,
            id: uuid(),
          },
          {
            role: MessageType.Assistant,
            content: answer,
            id: uuid(),
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
          id: uuid(),
          role: MessageType.Assistant,
          content: answer.answer,
          reference: answer.reference,
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

export const useSendMessage = (
  addNewestQuestion: (message: string, answer?: string) => void,
  removeLatestMessage: () => void,
  addNewestAnswer: (answer: IAnswer) => void,
) => {
  const { id: flowId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { refetch } = useFetchFlow();

  const { send, answer, done } = useSendMessageWithSse(api.runCanvas);

  const sendMessage = useCallback(
    async (message: string) => {
      const params: Record<string, unknown> = {
        id: flowId,
      };
      if (message) {
        params.message = message;
      }
      const res = await send(params);

      if (receiveMessageError(res)) {
        antMessage.error(res?.data?.retmsg);

        // cancel loading
        setValue(message);
        removeLatestMessage();
      } else {
        refetch(); // pull the message list after sending the message successfully
      }
    },
    [flowId, removeLatestMessage, setValue, send, refetch],
  );

  const handleSendMessage = useCallback(
    async (message: string) => {
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
    if (done) {
      setValue('');
      handleSendMessage(value.trim());
    }
    addNewestQuestion(value);
  }, [addNewestQuestion, handleSendMessage, done, setValue, value]);

  return {
    handlePressEnter,
    handleInputChange,
    value,
    loading: !done,
  };
};
