import { NextMessageInputOnPressEnterParameter } from '@/components/message-input/next';
import { MessageType } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useScrollToBottom,
} from '@/hooks/logic-hooks';
import { useFetchChat, useGetChatSearchParams } from '@/hooks/use-chat-request';
import { buildMessageListWithUuid } from '@/utils/chat';
import { IMessage, Message } from '@/interfaces/database/chat';
import notification from '@/utils/notification';
import { trim } from 'lodash';
import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { v4 as uuid } from 'uuid';
import { runChatCompletionStream } from '../chat-stream/run-stream';
import {
  useChatPendingInput,
  useChatStreamMessages,
  useChatStreamStore,
  useIsChatStreaming,
} from '../chat-stream/store';
import { resolveResendOptions } from '../utils';
import { useCreateConversationBeforeSendMessage } from './use-chat-url';
import { useFindPrologueFromDialogList } from './use-select-conversation-list';
import { useUploadFile } from './use-upload-file';

/**
 * Subscribes to the current conversation's entry in the chat stream store and
 * seeds the prologue for a brand new session. Because the store lives at module
 * level, an in-flight stream keeps updating it while this page is unmounted, so
 * switching back re-attaches to the still-running stream.
 */
export const useCurrentChatSession = () => {
  const { conversationId, isNew } = useGetChatSearchParams();
  const { id: dialogId } = useParams();
  const prologue = useFindPrologueFromDialogList();

  const messages = useChatStreamMessages(conversationId);
  const isStreaming = useIsChatStreaming(conversationId);

  const messageContainerRef = useRef<HTMLDivElement>(null);
  const { scrollRef } = useScrollToBottom(messages, messageContainerRef);

  const ensureSession = useChatStreamStore((state) => state.ensureSession);
  const seedPrologue = useChatStreamStore((state) => state.seedPrologue);

  useEffect(() => {
    if (!conversationId) return;
    ensureSession(conversationId, dialogId ?? '');
  }, [conversationId, dialogId, ensureSession]);

  useEffect(() => {
    if (!conversationId || isNew !== 'true') return;
    seedPrologue(conversationId, dialogId ?? '', prologue ?? '');
  }, [conversationId, dialogId, isNew, prologue, seedPrologue]);

  return {
    messages,
    isStreaming,
    scrollRef,
    messageContainerRef,
  };
};

export const useSendMessage = () => {
  const { conversationId } = useGetChatSearchParams();
  const { t } = useTranslation();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { handleUploadFile, isUploading, removeFile, files, clearFiles } =
    useUploadFile();

  const { id: chatId } = useParams();
  const { data: currentDialog } = useFetchChat();
  const { messages, isStreaming, scrollRef, messageContainerRef } =
    useCurrentChatSession();

  const appendQuestion = useChatStreamStore((state) => state.appendQuestion);
  const failStream = useChatStreamStore((state) => state.failStream);
  const consumePendingInput = useChatStreamStore(
    (state) => state.consumePendingInput,
  );
  const pendingInput = useChatPendingInput(conversationId);
  const removeMessageFromStore = useChatStreamStore(
    (state) => state.removeMessageById,
  );
  const hydrateFromServer = useChatStreamStore(
    (state) => state.hydrateFromServer,
  );
  const stopStream = useChatStreamStore((state) => state.stopStream);

  // The regenerate button lives in the transcript, which has no access to the
  // input box's thinking / internet toggles. Remember what the last send used
  // so a retry keeps the same options instead of silently dropping them.
  const lastSendOptionsRef = useRef<NextMessageInputOnPressEnterParameter>({});

  const sendMessage = useCallback(
    async ({
      message,
      currentConversationId,
      messages: explicitMessages,
      enableInternet,
      enableThinking,
    }: {
      message: IMessage;
      currentConversationId?: string;
      messages?: IMessage[];
    } & NextMessageInputOnPressEnterParameter) => {
      const sessionId = currentConversationId ?? conversationId;

      lastSendOptionsRef.current = { enableInternet, enableThinking };

      const { ok, aborted } = await runChatCompletionStream({
        conversationId: sessionId,
        chatId,
        // An explicitly provided list is authoritative, even when empty
        // (e.g. regenerating the first question must truncate history).
        messages: [
          ...(Array.isArray(explicitMessages) ? explicitMessages : messages),
          message,
        ],
        enableThinking,
        enableInternet,
        llmSetting: currentDialog?.llm_setting,
      });

      if (!ok && !aborted) {
        // The failure can land long after the page unmounted or after the user
        // switched conversations, so don't write to the input box here: park the
        // text on the failing session instead, and tell the user something went
        // wrong wherever they currently are.
        failStream(sessionId, message.content);
        notification.error({ message: t('message.requestError') });
      }
    },
    [
      conversationId,
      chatId,
      messages,
      failStream,
      t,
      currentDialog?.llm_setting,
    ],
  );

  // Hand a failed question back to the input box, but only once the box is
  // empty so a message the user is currently typing is never clobbered.
  useEffect(() => {
    if (!pendingInput || trim(value) !== '') return;
    consumePendingInput(conversationId);
    setValue(pendingInput);
  }, [pendingInput, value, conversationId, consumePendingInput, setValue]);

  const removeMessageById = useCallback(
    (messageId: string) => {
      removeMessageFromStore(conversationId, messageId);
    },
    [conversationId, removeMessageFromStore],
  );

  // Regenerating re-asks the same question as a new turn instead of rewriting
  // the original exchange, so every earlier answer stays in the transcript.
  const regenerateMessage = useCallback(
    (message: Message) => {
      if (isStreaming) return;

      const questionMessage: IMessage = {
        content: message.content,
        files: message.files,
        id: uuid(),
        role: MessageType.User,
        conversationId,
      };

      appendQuestion(conversationId, questionMessage);
      sendMessage({
        message: questionMessage,
        ...resolveResendOptions(lastSendOptionsRef.current),
      });
    },
    [conversationId, isStreaming, appendQuestion, sendMessage],
  );

  const { createConversationBeforeSendMessage } =
    useCreateConversationBeforeSendMessage();

  const handlePressEnter = useCallback(
    async ({
      enableThinking,
      enableInternet,
    }: NextMessageInputOnPressEnterParameter) => {
      if (trim(value) === '' || isStreaming) return;

      const data = await createConversationBeforeSendMessage(value);

      if (data === undefined) {
        return;
      }

      const { targetConversationId, currentMessages } = data;

      // The await above can span a conversation switch, and the target session
      // may already have a stream running.
      if (
        useChatStreamStore.getState().sessions[targetConversationId]
          ?.isStreaming
      ) {
        return;
      }

      const id = uuid();

      // useUploadFile tracks files as loosely typed records; narrow once here.
      const attachedFiles = files as IMessage['files'];

      const questionMessage: IMessage = {
        content: value,
        files: attachedFiles,
        id,
        role: MessageType.User,
        conversationId: targetConversationId,
      };

      // A brand new conversation is persisted under a server-generated id, so
      // the prologue seeded on the temporary id doesn't carry over. Seed the
      // real session from the server's list (which holds just the prologue)
      // BEFORE appending the question — hydrating afterwards would pass the
      // authority check (not streaming yet) and wipe the question and
      // placeholder that were just appended.
      if (currentMessages.length > 0) {
        hydrateFromServer(
          targetConversationId,
          buildMessageListWithUuid(currentMessages),
        );
      }

      // Route the question to the conversation it was asked in, not whichever
      // is currently displayed.
      //
      // Snapshot the history before appendQuestion writes the question and its
      // assistant placeholder into the store.
      const history =
        useChatStreamStore.getState().sessions[targetConversationId]
          ?.messages ?? [];

      appendQuestion(targetConversationId, questionMessage);

      setValue('');
      sendMessage({
        currentConversationId: targetConversationId,
        // For an existing conversation currentMessages is empty; fall back to
        // the store's message list instead of sending an empty history.
        messages: currentMessages.length > 0 ? currentMessages : history,
        message: {
          id,
          content: value.trim(),
          role: MessageType.User,
          files: attachedFiles,
          conversationId: targetConversationId,
        },
        enableInternet,
        enableThinking,
      });

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
      isStreaming,
      createConversationBeforeSendMessage,
      appendQuestion,
      hydrateFromServer,
      files,
      clearFiles,
      setValue,
      sendMessage,
      messageContainerRef,
    ],
  );

  const stopOutputMessage = useCallback(() => {
    stopStream(conversationId);
  }, [conversationId, stopStream]);

  return {
    handlePressEnter,
    handleInputChange,
    value,
    setValue,
    regenerateMessage,
    sendLoading: isStreaming,
    scrollRef,
    messageContainerRef,
    messages,
    removeMessageById,
    handleUploadFile,
    isUploading,
    removeFile,
    hydrateFromServer,
    stopOutputMessage,
  };
};
