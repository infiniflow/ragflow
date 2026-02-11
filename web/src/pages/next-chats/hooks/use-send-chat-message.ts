import { NextMessageInputOnPressEnterParameter } from '@/components/message-input/next';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useRegenerateMessage,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IMessage } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { trim } from 'lodash';
import { useCallback, useEffect } from 'react';
import { useParams } from 'react-router';
import { v4 as uuid } from 'uuid';
import { useCreateConversationBeforeSendMessage } from './use-chat-url';
import { useFindPrologueFromDialogList } from './use-select-conversation-list';
import { useUploadFile } from './use-upload-file';

export const useSelectNextMessages = () => {
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
  const { isNew, conversationId } = useGetChatSearchParams();
  const { id: dialogId } = useParams();
  const prologue = useFindPrologueFromDialogList();

  const addPrologue = useCallback(() => {
    if (dialogId !== '' && isNew === 'true') {
      const nextMessage = {
        role: MessageType.Assistant,
        content: prologue,
        id: uuid(),
        conversationId: conversationId,
      } as IMessage;

      setDerivedMessages([nextMessage]);
    }
  }, [conversationId, dialogId, isNew, prologue, setDerivedMessages]);

  useEffect(() => {
    addPrologue();
  }, [addPrologue]);

  return {
    scrollRef,
    messageContainerRef,
    derivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
    removeMessageById,
    removeMessagesAfterCurrentMessage,
    setDerivedMessages,
  };
};

export const useSendMessage = (controller: AbortController) => {
  const { conversationId, isNew } = useGetChatSearchParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { handleUploadFile, isUploading, removeFile, files, clearFiles } =
    useUploadFile();

  const { send, answer, done } = useSendMessageWithSse(
    api.completeConversation,
  );
  const {
    scrollRef,
    messageContainerRef,
    derivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
    removeMessageById,
    removeMessagesAfterCurrentMessage,
    setDerivedMessages,
  } = useSelectNextMessages();

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

  const { regenerateMessage } = useRegenerateMessage({
    removeMessagesAfterCurrentMessage,
    sendMessage,
    messages: derivedMessages,
  });

  const { createConversationBeforeSendMessage } =
    useCreateConversationBeforeSendMessage();

  const handlePressEnter = useCallback(
    async ({
      enableThinking,
      enableInternet,
    }: NextMessageInputOnPressEnterParameter) => {
      if (trim(value) === '') return;

      const data = await createConversationBeforeSendMessage(value);

      if (data === undefined) {
        return;
      }

      const { targetConversationId, currentMessages } = data;

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
    [
      value,
      createConversationBeforeSendMessage,
      addNewestQuestion,
      files,
      done,
      clearFiles,
      setValue,
      sendMessage,
    ],
  );

  useEffect(() => {
    //  #1289
    if (answer.answer && conversationId && isNew !== 'true') {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer, conversationId, isNew]);

  return {
    handlePressEnter,
    handleInputChange,
    value,
    setValue,
    regenerateMessage,
    sendLoading: !done,
    scrollRef,
    messageContainerRef,
    derivedMessages,
    removeMessageById,
    handleUploadFile,
    isUploading,
    removeFile,
    setDerivedMessages,
  };
};
