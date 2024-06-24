import { MessageType } from '@/constants/chat';
import { useFetchFlow } from '@/hooks/flow-hooks';
import {
  useHandleMessageInputChange,
  useScrollToBottom,
  useSendMessageWithSse,
} from '@/hooks/logicHooks';
import { IAnswer } from '@/interfaces/database/chat';
import { IMessage } from '@/pages/chat/interface';
import api from '@/utils/api';
import { message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'umi';
import { v4 as uuid } from 'uuid';

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
      const latestMessage = pre?.at(-1);

      if (latestMessage) {
        return [
          ...pre.slice(0, -1),
          {
            ...latestMessage,
            content: answer.answer,
            reference: answer.reference,
          },
        ];
      }
      return pre;
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
  const { data: flowDetail } = useFetchFlow();
  const messages = flowDetail.dsl.messages;

  const { send, answer, done } = useSendMessageWithSse(api.runCanvas);

  const sendMessage = useCallback(
    async (message: string, id?: string) => {
      const params: Record<string, unknown> = {
        id: flowId,
      };
      if (message) {
        params.message = message;
      }
      const res = await send(params);

      if (res && (res?.response.status !== 200 || res?.data?.retcode !== 0)) {
        antMessage.error(res?.data?.retmsg);

        // cancel loading
        setValue(message);
        removeLatestMessage();
      }
    },
    [flowId, removeLatestMessage, setValue, send],
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

  useEffect(() => {
    // fetch prologue
    if (messages.length === 0) {
      sendMessage('');
    }
  }, [sendMessage, messages]);

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
