import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IAnswer, Message } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { buildMessageUuid } from '@/utils/chat';
import { trim } from 'lodash';
import { useCallback, useEffect, useState } from 'react';
import { v4 as uuid } from 'uuid';
import { IMessage } from '../chat/interface';
import { useBuildFormRefs } from './use-build-form-refs';
import { useUploadFile } from './use-upload-file';

export function useSendMultipleChatMessage(
  controller: AbortController,
  chatBoxIds: string[],
) {
  const [messageRecord, setMessageRecord] = useState<
    Record<string, IMessage[]>
  >({});

  const { conversationId } = useGetChatSearchParams();

  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { send, answer, allDone } = useSendMessageWithSse(
    api.completeConversation,
  );

  const { handleUploadFile, fileIds, clearFileIds } = useUploadFile();

  const { setFormRef, getLLMConfigById, isLLMConfigEmpty } =
    useBuildFormRefs(chatBoxIds);

  const stopOutputMessage = useCallback(() => {
    controller.abort();
  }, [controller]);

  const addNewestQuestion = useCallback(
    (message: Message, answer: string = '') => {
      setMessageRecord((pre) => {
        const currentRecord = { ...pre };
        const chatBoxId = message.chatBoxId;
        if (typeof chatBoxId === 'string') {
          const currentChatMessages = currentRecord[chatBoxId];

          const nextChatMessages = [
            ...currentChatMessages,
            {
              ...message,
              id: buildMessageUuid(message), // The message id is generated on the front end,
              // and the message id returned by the back end is the same as the question id,
              //  so that the pair of messages can be deleted together when deleting the message
            },
            {
              role: MessageType.Assistant,
              content: answer,
              id: buildMessageUuid({ ...message, role: MessageType.Assistant }),
            },
          ];

          currentRecord[chatBoxId] = nextChatMessages;
        }

        return currentRecord;
      });
    },
    [],
  );

  // Add the streaming message to the last item in the message list
  const addNewestAnswer = useCallback((answer: IAnswer) => {
    setMessageRecord((pre) => {
      const currentRecord = { ...pre };
      const chatBoxId = answer.chatBoxId;
      if (typeof chatBoxId === 'string') {
        const currentChatMessages = currentRecord[chatBoxId];

        const nextChatMessages = [
          ...(currentChatMessages?.slice(0, -1) ?? []),
          {
            role: MessageType.Assistant,
            content: answer.answer,
            reference: answer.reference,
            id: buildMessageUuid({
              id: answer.id,
              role: MessageType.Assistant,
            }),
            prompt: answer.prompt,
            audio_binary: answer.audio_binary,
          },
        ];

        currentRecord[chatBoxId] = nextChatMessages;
      }

      return currentRecord;
    });
  }, []);

  const removeLatestMessage = useCallback((chatBoxId?: string) => {
    setMessageRecord((pre) => {
      const currentRecord = { ...pre };
      if (chatBoxId) {
        const currentChatMessages = currentRecord[chatBoxId];
        if (currentChatMessages) {
          currentRecord[chatBoxId] = currentChatMessages.slice(0, -1);
        }
      }
      return currentRecord;
    });
  }, []);

  const adjustRecordByChatBoxIds = useCallback(() => {
    setMessageRecord((pre) => {
      const currentRecord = { ...pre };
      chatBoxIds.forEach((chatBoxId) => {
        if (!currentRecord[chatBoxId]) {
          currentRecord[chatBoxId] = [];
        }
      });
      Object.keys(currentRecord).forEach((chatBoxId) => {
        if (!chatBoxIds.includes(chatBoxId)) {
          delete currentRecord[chatBoxId];
        }
      });
      return currentRecord;
    });
  }, [chatBoxIds, setMessageRecord]);

  const sendMessage = useCallback(
    async ({
      message,
      currentConversationId,
      messages,
      chatBoxId,
    }: {
      message: Message;
      currentConversationId?: string;
      chatBoxId: string;
      messages?: Message[];
    }) => {
      let derivedMessages: IMessage[] = [];

      derivedMessages = messageRecord[chatBoxId];

      const res = await send(
        {
          chatBoxId,
          conversation_id: currentConversationId ?? conversationId,
          messages: [...(messages ?? derivedMessages ?? []), message],
          ...getLLMConfigById(chatBoxId),
        },
        controller,
      );

      if (res && (res?.response.status !== 200 || res?.data?.code !== 0)) {
        // cancel loading
        setValue(message.content);
        console.info('removeLatestMessage111');
        removeLatestMessage(chatBoxId);
      }
    },
    [
      send,
      conversationId,
      getLLMConfigById,
      controller,
      messageRecord,
      setValue,
      removeLatestMessage,
    ],
  );

  const handlePressEnter = useCallback(() => {
    if (trim(value) === '') return;
    const id = uuid();

    chatBoxIds.forEach((chatBoxId) => {
      if (!isLLMConfigEmpty(chatBoxId)) {
        addNewestQuestion({
          content: value,
          id,
          role: MessageType.User,
          chatBoxId,
          doc_ids: fileIds,
        });
      }
    });

    if (allDone) {
      setValue('');
      chatBoxIds.forEach((chatBoxId) => {
        if (!isLLMConfigEmpty(chatBoxId)) {
          sendMessage({
            message: {
              id,
              content: value.trim(),
              role: MessageType.User,
              doc_ids: fileIds,
            },
            chatBoxId,
          });
        }
      });
    }
    clearFileIds();
  }, [
    value,
    chatBoxIds,
    allDone,
    clearFileIds,
    isLLMConfigEmpty,
    addNewestQuestion,
    fileIds,
    setValue,
    sendMessage,
  ]);

  useEffect(() => {
    if (answer.answer && conversationId) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer, conversationId]);

  useEffect(() => {
    adjustRecordByChatBoxIds();
  }, [adjustRecordByChatBoxIds]);

  return {
    value,
    messageRecord,
    sendMessage,
    handleInputChange,
    handlePressEnter,
    stopOutputMessage,
    sendLoading: !allDone,
    setFormRef,
    handleUploadFile,
  };
}
