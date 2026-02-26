import { NextMessageInputOnPressEnterParameter } from '@/components/message-input/next';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IMessage } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { useCallback, useEffect } from 'react';
import { v4 as uuid } from 'uuid';
import { CreateConversationBeforeSendMessageReturnType } from './use-chat-url';
import { useUploadFile } from './use-upload-file';

export type UseSendSingleMessageParameter = {
  controller: AbortController;
} & Pick<ReturnType<typeof useHandleMessageInputChange>, 'value' | 'setValue'> &
  Pick<ReturnType<typeof useUploadFile>, 'files' | 'clearFiles'>;

export function useSendSingleMessage({
  controller,
  value,
  setValue,
  files,
  clearFiles,
}: {
  controller: AbortController;
} & Pick<ReturnType<typeof useHandleMessageInputChange>, 'value' | 'setValue'> &
  Pick<ReturnType<typeof useUploadFile>, 'files' | 'clearFiles'>) {
  const { conversationId } = useGetChatSearchParams();

  const { send, answer, done } = useSendMessageWithSse(
    api.completeConversation,
  );

  const {
    scrollRef,
    messageContainerRef,
    setDerivedMessages,
    derivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
    removeMessageById,
    removeMessagesAfterCurrentMessage,
  } = useSelectDerivedMessages();

  useEffect(() => {
    if (answer.answer) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer]);

  const sendMessage = useCallback(
    async ({
      message,
      currentConversationId,
      messages,
      enableInternet,
      enableThinking,
    }: {
      message: IMessage;
      currentConversationId?: string;
      messages?: IMessage[];
    } & NextMessageInputOnPressEnterParameter) => {
      const res = await send(
        {
          conversation_id: currentConversationId ?? conversationId,
          messages: [
            ...(Array.isArray(messages) && messages?.length > 0
              ? messages
              : (derivedMessages ?? [])),
            message,
          ],
          reasoning: enableThinking,
          internet: enableInternet,
        },
        controller,
      );

      if (res && (res?.response.status !== 200 || res?.data?.code !== 0)) {
        // cancel loading
        setValue(message.content);
        console.info('removeLatestMessage111');
        removeLatestMessage();
      }
    },
    [
      derivedMessages,
      conversationId,
      removeLatestMessage,
      setValue,
      send,
      controller,
    ],
  );

  const handlePressEnter = useCallback(
    async ({
      enableThinking,
      enableInternet,
      currentMessages,
      targetConversationId,
    }: NextMessageInputOnPressEnterParameter &
      CreateConversationBeforeSendMessageReturnType) => {
      const id = uuid();

      addNewestQuestion({
        content: value,
        files: files,
        id,
        role: MessageType.User,
        conversationId: targetConversationId,
      });

      if (done) {
        setValue('');
        sendMessage({
          currentConversationId: targetConversationId,
          messages: currentMessages,
          message: {
            id,
            content: value.trim(),
            role: MessageType.User,
            files: files,
            conversationId: targetConversationId,
          },
          enableInternet,
          enableThinking,
        });
      }
      clearFiles();
    },
    [addNewestQuestion, value, files, done, clearFiles, setValue, sendMessage],
  );

  return {
    scrollRef,
    messageContainerRef,
    setDerivedMessages,
    derivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
    removeMessageById,
    removeMessagesAfterCurrentMessage,
    handlePressEnter,
  };
}

export type HandlePressEnterType = ReturnType<
  typeof useSendSingleMessage
>['handlePressEnter'];
