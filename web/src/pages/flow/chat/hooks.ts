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
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { Operator, initialBeginValues } from '../constant';
import useGraphStore from '../store';

export const useSelectCurrentMessages = () => {
  const { id: id } = useParams();
  const findNodeByName = useGraphStore((state) => state.findNodeByName);
  const [currentMessages, setCurrentMessages] = useState<IMessage[]>([]);

  const { data: flowDetail } = useFetchFlow();
  const messages = flowDetail.dsl.messages;
  const reference = flowDetail.dsl.reference;

  const ref = useScrollToBottom(currentMessages);

  const prologue =
    findNodeByName(Operator.Begin)?.data?.form?.prologue ??
    initialBeginValues.prologue;

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
      return [...pre, ...nextMessages];
    });
  }, []);

  // const addPrologue = useCallback(() => {
  //   if (messages.length === 0) {
  //     const nextMessage = {
  //       role: MessageType.Assistant,
  //       content: prologue,
  //       id: uuid(),
  //     } as IMessage;

  //     setCurrentMessages([nextMessage]);
  //   }
  // }, [prologue, messages]);

  // useEffect(() => {
  //   addPrologue();
  // }, [addPrologue]);

  useEffect(() => {
    if (id) {
      const nextMessages = messages.map((x) => ({ ...x, id: uuid() }));
      if (messages.length === 0) {
        nextMessages.unshift({
          role: MessageType.Assistant,
          content: prologue,
          id: uuid(),
        });
      }
      setCurrentMessages(nextMessages);
    }
  }, [messages, id, prologue]);

  return {
    currentMessages,
    reference,
    addNewestQuestion,
    removeLatestMessage,
    addNewestAnswer,
    ref,
  };
};

export const useSendMessage = (
  addNewestQuestion: (message: string, answer?: string) => void,
  removeLatestMessage: () => void,
  addNewestAnswer: (answer: IAnswer) => void,
) => {
  const { id: flowId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { send, answer, done } = useSendMessageWithSse(api.runCanvas);

  const sendMessage = useCallback(
    async (message: string, id?: string) => {
      const res: Response | undefined = await send({ id: flowId, message });

      if (res?.status !== 200) {
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
