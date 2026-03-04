import { NextMessageInputOnPressEnterParameter } from '@/components/message-input/next';
import showMessage from '@/components/ui/message';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IAnswer, IMessage, Message } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { buildMessageUuid } from '@/utils/chat';
import { trim } from 'lodash';
import { useCallback, useEffect, useState } from 'react';
import { v4 as uuid } from 'uuid';
import { useBuildFormRefs } from './use-build-form-refs';
import { useCreateConversationBeforeSendMessage } from './use-chat-url';
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

  const { handleUploadFile, isUploading, files, clearFiles, removeFile } =
    useUploadFile();

  const { createConversationBeforeSendMessage } =
    useCreateConversationBeforeSendMessage();

  const { setFormRef, getLLMConfigById, isLLMConfigEmpty } =
    useBuildFormRefs(chatBoxIds);

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
      enableInternet,
      enableThinking,
    }: {
      message: Message;
      currentConversationId?: string;
      chatBoxId: string;
      messages?: Message[];
    } & NextMessageInputOnPressEnterParameter) => {
      let derivedMessages: IMessage[] = [];

      derivedMessages = messageRecord[chatBoxId];

      const res = await send(
        {
          chatBoxId,
          conversation_id: currentConversationId ?? conversationId,
          messages: [...(messages ?? derivedMessages ?? []), message],
          reasoning: enableThinking,
          internet: enableInternet,
          ...getLLMConfigById(chatBoxId),
        },
        controller,
      );

      if (res && (res?.response.status !== 200 || res?.data?.code !== 0)) {
        // cancel loading
        setValue(message.content);
        showMessage.error(res.data.message);
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

  const handlePressEnter = useCallback(
    async ({
      enableThinking,
      enableInternet,
    }: NextMessageInputOnPressEnterParameter) => {
      if (trim(value) === '') return;
      const id = uuid();

      const data = await createConversationBeforeSendMessage(value);

      if (data === undefined) {
        return;
      }

      const { targetConversationId, currentMessages } = data;

      chatBoxIds.forEach((chatBoxId) => {
        if (!isLLMConfigEmpty(chatBoxId)) {
          addNewestQuestion({
            content: value,
            id,
            role: MessageType.User,
            chatBoxId,
            files,
            conversationId: targetConversationId,
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
                files,
                conversationId: targetConversationId,
              },
              chatBoxId,
              currentConversationId: targetConversationId,
              messages: currentMessages,
              enableThinking,
              enableInternet,
            });
          }
        });
      }
      clearFiles();
    },
    [
      value,
      createConversationBeforeSendMessage,
      chatBoxIds,
      allDone,
      clearFiles,
      isLLMConfigEmpty,
      addNewestQuestion,
      files,
      setValue,
      sendMessage,
    ],
  );

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
    sendLoading: !allDone,
    setFormRef,
    handleUploadFile,
    isUploading,
    removeFile,
  };
}
