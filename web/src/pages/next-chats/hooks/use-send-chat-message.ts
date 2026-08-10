import { NextMessageInputOnPressEnterParameter } from '@/components/message-input/next';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useRegenerateMessage,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IAnswer, IMessage } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { buildMessageUuid } from '@/utils/chat';
import { omit, trim } from 'lodash';
import { useCallback, useEffect, useRef } from 'react';
import { useParams } from 'react-router';
import { v4 as uuid } from 'uuid';
import { useCreateConversationBeforeSendMessage } from './use-chat-url';
import { useFindPrologueFromDialogList } from './use-select-conversation-list';
import { useUploadFile } from './use-upload-file';

// Append an answer to a messages array (mirrors addNewestAnswer logic in
// useSelectDerivedMessages) for updating a background conversation's cache
// without touching the currently displayed derivedMessages.
function appendAnswerToMessages(
  messages: IMessage[],
  answer: IAnswer,
): IMessage[] {
  return [
    ...(messages.slice(0, -1) ?? []),
    {
      role: MessageType.Assistant,
      content: answer.answer,
      reference: answer.reference,
      id: buildMessageUuid({ id: answer.id, role: MessageType.Assistant }),
      prompt: answer.prompt,
      audio_binary: answer.audio_binary,
      ...omit(answer, 'reference'),
    },
  ];
}

// Append a user question + empty assistant placeholder to a messages array
// (mirrors addNewestQuestion logic) for updating a background conversation's
// cache when the user asks a question right before switching conversations.
function appendQuestionToMessages(
  messages: IMessage[],
  message: IMessage,
): IMessage[] {
  return [
    ...messages,
    {
      ...message,
      id: buildMessageUuid(message),
    },
    {
      role: MessageType.Assistant,
      content: '',
      conversationId: message.conversationId,
      id: buildMessageUuid({ ...message, role: MessageType.Assistant }),
    },
  ];
}

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

  // Per-conversation message cache so switching conversations preserves the
  // local derivedMessages (including in-flight SSE answers) instead of wiping
  // them. Keyed by conversationId.
  const messagesCacheRef = useRef<Record<string, IMessage[]>>({});
  // Tracks conversations with an in-flight SSE stream so the backend sync
  // effect in SingleChatBox can avoid overwriting local streaming state.
  const activeStreamsRef = useRef<Set<string>>(new Set());
  // Tracks which conversationId the current derivedMessages belongs to, so
  // the cache-write effect doesn't mistakenly write the previous conversation's
  // messages into the newly switched conversation's cache entry during the
  // render where conversationId has changed but derivedMessages hasn't yet.
  const messagesConversationIdRef = useRef(conversationId);

  // On conversation switch, restore cached messages (or start empty).
  useEffect(() => {
    messagesConversationIdRef.current = conversationId;
    const cached = messagesCacheRef.current[conversationId];
    setDerivedMessages(cached ?? []);
  }, [conversationId, setDerivedMessages]);

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

  // Persist current derivedMessages back into the cache for the conversation
  // they actually belong to (not necessarily the URL's current conversationId
  // during a switch).
  useEffect(() => {
    messagesCacheRef.current[messagesConversationIdRef.current] = derivedMessages;
  }, [derivedMessages]);

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
    messagesCacheRef,
    activeStreamsRef,
  };
};

export const useSendMessage = (controller: AbortController) => {
  const { conversationId, isNew } = useGetChatSearchParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { handleUploadFile, isUploading, removeFile, files, clearFiles } =
    useUploadFile();

  const { id: chatId } = useParams();
  const { send, answer, done } = useSendMessageWithSse();
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
    messagesCacheRef,
    activeStreamsRef,
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
      const sessionId = currentConversationId ?? conversationId;
      activeStreamsRef.current.add(sessionId);
      try {
        const res = await send(
          api.completionUrl,
          {
            chat_id: chatId,
            session_id: sessionId,
            // An explicitly provided list is authoritative, even when empty
            // (e.g. regenerating the first question must truncate history).
            messages: [
              ...(Array.isArray(messages) ? messages : (derivedMessages ?? [])),
              message,
            ],
            pass_all_history_messages: true,
            reasoning: Number(enableThinking),
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
      } finally {
        activeStreamsRef.current.delete(sessionId);
      }
    },
    [
      derivedMessages,
      conversationId,
      chatId,
      removeLatestMessage,
      setValue,
      send,
      controller,
      activeStreamsRef,
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
      if (trim(value) === '' || !done) return;

      const data = await createConversationBeforeSendMessage(value);

      if (data === undefined) {
        return;
      }

      const { targetConversationId, currentMessages } = data;

      const id = uuid();

      const questionMessage = {
        content: value,
        files: files,
        id,
        role: MessageType.User,
        conversationId: targetConversationId,
      };
      // The await on createConversationBeforeSendMessage above can span a
      // conversation switch. Route the question to the conversation it was
      // asked in, not whichever is currently displayed.
      if (targetConversationId === conversationId) {
        addNewestQuestion(questionMessage);
      } else {
        const cached = messagesCacheRef.current[targetConversationId] ?? [];
        messagesCacheRef.current[targetConversationId] =
          appendQuestionToMessages(cached, questionMessage);
      }

      if (done) {
        setValue('');
        sendMessage({
          currentConversationId: targetConversationId,
          // For an existing conversation currentMessages is empty; fall back
          // to derivedMessages instead of sending an empty history.
          messages: currentMessages.length > 0 ? currentMessages : undefined,
          message: {
            id,
            content: value.trim(),
            role: MessageType.User,
            files,
            conversationId: targetConversationId,
          },
          enableInternet,
          enableThinking,
        });
      }

      clearFiles();

      // Auto scroll to bottom when sending new message
      if (messageContainerRef.current) {
        const el = messageContainerRef.current;

        requestAnimationFrame(() => {
          el.scrollTo({
            top: el.scrollHeight,
          });
        });
      }
    },
    [
      value,
      done,
      createConversationBeforeSendMessage,
      addNewestQuestion,
      files,
      clearFiles,
      setValue,
      sendMessage,
      messageContainerRef,
      conversationId,
      messagesCacheRef,
    ],
  );

  useEffect(() => {
    //  #1289
    if (!answer.answer || isNew === 'true') return;
    // Route the answer to the conversation it belongs to. answer.conversationId
    // comes from the request body's session_id (set in useSendMessageWithSse.send),
    // NOT from the URL, so it stays correct even after switching conversations.
    // This prevents a background stream's answer from leaking into the currently
    // displayed conversation, and keeps the background conversation's cache
    // updating so switching back shows the streamed answer.
    const targetId = answer.conversationId;
    if (!targetId) return;
    if (targetId === conversationId) {
      addNewestAnswer(answer);
    } else {
      const cached = messagesCacheRef.current[targetId] ?? [];
      messagesCacheRef.current[targetId] =
        appendAnswerToMessages(cached, answer);
    }
  }, [answer, addNewestAnswer, conversationId, isNew, messagesCacheRef]);

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
    messagesCacheRef,
    activeStreamsRef,
  };
};
