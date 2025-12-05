import { FileUploadProps } from '@/components/file-upload';
import { ChatSearchParams, MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useRegenerateMessage,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IMessage } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { generateConversationId } from '@/utils/chat';
import { trim } from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';
import { useParams, useSearchParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { useFindPrologueFromDialogList } from './use-select-conversation-list';
import { useSetConversation } from './use-set-conversation';
import { useUploadFile } from './use-upload-file';

/**
 * Consolidated hook for managing chat URL parameters (conversationId and isNew)
 * Replaces: useClickConversationCard from use-chat-request.ts and useSetChatRouteParams from use-set-chat-route.ts
 */
export const useChatUrlParams = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );

  const setConversationId = useCallback(
    (conversationId: string) => {
      newQueryParameters.set(ChatSearchParams.ConversationId, conversationId);
      setSearchParams(newQueryParameters);
    },
    [setSearchParams, newQueryParameters],
  );

  const setIsNew = useCallback(
    (isNew: string) => {
      newQueryParameters.set(ChatSearchParams.isNew, isNew);
      setSearchParams(newQueryParameters);
    },
    [setSearchParams, newQueryParameters],
  );

  const getIsNew = useCallback(() => {
    return newQueryParameters.get(ChatSearchParams.isNew);
  }, [newQueryParameters]);

  const setConversationBoth = useCallback(
    (conversationId: string, isNew: string) => {
      newQueryParameters.set(ChatSearchParams.ConversationId, conversationId);
      newQueryParameters.set(ChatSearchParams.isNew, isNew);
      setSearchParams(newQueryParameters);
    },
    [setSearchParams, newQueryParameters],
  );

  return {
    setConversationId,
    setIsNew,
    getIsNew,
    setConversationBoth,
  };
};

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
  const { setConversation } = useSetConversation();
  const { conversationId, isNew } = useGetChatSearchParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { setIsNew, setConversationBoth } = useChatUrlParams();

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

  const onUploadFile: NonNullable<FileUploadProps['onUpload']> = useCallback(
    async (files, options) => {
      if (
        (conversationId === '' || isNew === 'true') &&
        Array.isArray(files) &&
        files.length
      ) {
        const currentConversationId = generateConversationId();

        if (conversationId === '') {
          setConversationBoth(currentConversationId, 'true');
        }

        const data = await setConversation(
          files[0].name,
          true,
          conversationId || currentConversationId,
        );
        if (data.code === 0) {
          setIsNew('');
          handleUploadFile(files, options, data.data?.id);
        }
      } else {
        handleUploadFile(files, options);
      }
    },
    [
      conversationId,
      handleUploadFile,
      isNew,
      setConversation,
      setConversationBoth,
      setIsNew,
    ],
  );

  const sendMessage = useCallback(
    async ({
      message,
      currentConversationId,
      messages,
    }: {
      message: IMessage;
      currentConversationId?: string;
      messages?: IMessage[];
    }) => {
      const res = await send(
        {
          conversation_id: currentConversationId ?? conversationId,
          messages: [
            ...(Array.isArray(messages) && messages?.length > 0
              ? messages
              : derivedMessages ?? []),
            message,
          ],
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

  const handlePressEnter = useCallback(async () => {
    if (trim(value) === '') return;

    let currentMessages: Array<IMessage> = [];
    const currentConversationId = generateConversationId();
    if (conversationId === '' || isNew === 'true') {
      if (conversationId === '') {
        setConversationBoth(currentConversationId, 'true');
      }
      const data = await setConversation(
        value,
        true,
        conversationId || currentConversationId,
      );
      if (data.code !== 0) {
        return;
      } else {
        setIsNew('');
        currentMessages = data.data.message;
      }
    }

    const id = uuid();

    addNewestQuestion({
      content: value,
      files: files,
      id,
      role: MessageType.User,
      conversationId: conversationId || currentConversationId,
    });

    if (done) {
      setValue('');
      sendMessage({
        currentConversationId: conversationId || currentConversationId,
        messages: currentMessages,
        message: {
          id,
          content: value.trim(),
          role: MessageType.User,
          files: files,
          conversationId: conversationId || currentConversationId,
        },
      });
    }
    clearFiles();
  }, [
    value,
    conversationId,
    isNew,
    addNewestQuestion,
    files,
    done,
    clearFiles,
    setConversation,
    setConversationBoth,
    setIsNew,
    setValue,
    sendMessage,
  ]);

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
    handleUploadFile: onUploadFile,
    isUploading,
    removeFile,
    setDerivedMessages,
  };
};
