import { MessageType } from '@/constants/chat';
import { useFetchFlow } from '@/hooks/flow-hooks';
import {
  useHandleMessageInputChange,
  //   useScrollToBottom,
  useSendMessageWithSse,
} from '@/hooks/logicHooks';
import { IAnswer } from '@/interfaces/database/chat';
import { IMessage } from '@/pages/chat/interface';
import omit from 'lodash/omit';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { Operator } from '../constant';
import useGraphStore from '../store';

export const useSelectCurrentConversation = () => {
  const { id: id } = useParams();
  const findNodeByName = useGraphStore((state) => state.findNodeByName);
  const [currentMessages, setCurrentMessages] = useState<IMessage[]>([]);

  const { data: flowDetail } = useFetchFlow();
  const messages = flowDetail.dsl.history;

  const prologue = findNodeByName(Operator.Begin)?.data?.form?.prologue;

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

  const addNewestAnswer = useCallback(
    (answer: IAnswer) => {
      setCurrentMessages((pre) => {
        const latestMessage = currentMessages?.at(-1);

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
    },
    [currentMessages],
  );

  const removeLatestMessage = useCallback(() => {
    setCurrentMessages((pre) => {
      const nextMessages = pre?.slice(0, -2) ?? [];
      return [...pre, ...nextMessages];
    });
  }, []);

  const addPrologue = useCallback(() => {
    if (id === '') {
      const nextMessage = {
        role: MessageType.Assistant,
        content: prologue,
        id: uuid(),
      } as IMessage;

      setCurrentMessages({
        id: '',
        reference: [],
        message: [nextMessage],
      } as any);
    }
  }, [id, prologue]);

  useEffect(() => {
    addPrologue();
  }, [addPrologue]);

  useEffect(() => {
    if (id) {
      setCurrentMessages(messages);
    }
  }, [messages, id]);

  return {
    currentConversation: currentMessages,
    addNewestQuestion,
    removeLatestMessage,
    addNewestAnswer,
  };
};

// export const useFetchConversationOnMount = () => {
//   const { conversationId } = useGetChatSearchParams();
//   const fetchConversation = useFetchConversation();
//   const {
//     currentConversation,
//     addNewestQuestion,
//     removeLatestMessage,
//     addNewestAnswer,
//   } = useSelectCurrentConversation();
//   const ref = useScrollToBottom(currentConversation);

//   const fetchConversationOnMount = useCallback(() => {
//     if (isConversationIdExist(conversationId)) {
//       fetchConversation(conversationId);
//     }
//   }, [fetchConversation, conversationId]);

//   useEffect(() => {
//     fetchConversationOnMount();
//   }, [fetchConversationOnMount]);

//   return {
//     currentConversation,
//     addNewestQuestion,
//     ref,
//     removeLatestMessage,
//     addNewestAnswer,
//   };
// };

export const useSendMessage = (
  conversation: any,
  addNewestQuestion: (message: string, answer?: string) => void,
  removeLatestMessage: () => void,
  addNewestAnswer: (answer: IAnswer) => void,
) => {
  const { id: conversationId } = useParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { send, answer, done } = useSendMessageWithSse();

  const sendMessage = useCallback(
    async (message: string, id?: string) => {
      const res: Response | undefined = await send({
        conversation_id: id ?? conversationId,
        messages: [
          ...(conversation?.message ?? []).map((x: IMessage) => omit(x, 'id')),
          {
            role: MessageType.User,
            content: message,
          },
        ],
      });

      if (res?.status !== 200) {
        // cancel loading
        setValue(message);
        removeLatestMessage();
      }
    },
    [
      conversation?.message,
      conversationId,
      removeLatestMessage,
      setValue,
      send,
    ],
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
