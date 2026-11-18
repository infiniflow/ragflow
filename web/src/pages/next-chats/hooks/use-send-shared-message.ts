import { NextMessageInputOnPressEnterParameter } from '@/components/message-input/next';
import message from '@/components/ui/message';
import { MessageType, SharedFrom } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useFetchExternalChatInfo } from '@/hooks/use-chat-request';
import { Message } from '@/interfaces/database/chat';
import trim from 'lodash/trim';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router';
import { v4 as uuid } from 'uuid';

const isCompletionError = (res: any) =>
  res &&
  (res?.response.status !== 200 ||
    (res?.data?.code !== undefined && res.data.code !== 0));

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();
  const data_prefix = 'data_';
  const serializedData = JSON.stringify(
    Array.from(searchParams.entries())
      .filter(([key]) => key.startsWith(data_prefix))
      .map(([key, value]) => [key.replace(data_prefix, ''), value])
      .sort(([left], [right]) => left.localeCompare(right)),
  );
  const data = useMemo(
    () => Object.fromEntries(JSON.parse(serializedData)),
    [serializedData],
  );
  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
    locale: searchParams.get('locale'),
    theme: searchParams.get('theme'),
    data: data,
    visibleAvatar: searchParams.get('visible_avatar')
      ? searchParams.get('visible_avatar') !== '1'
      : true,
  };
};

export const useSendSharedMessage = () => {
  const {
    from,
    sharedId: conversationId,
    data,
  } = useGetSharedChatSearchParams();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const completionUrl = `/api/v1/${from === SharedFrom.Agent ? 'agentbots' : 'chatbots'}/${conversationId}/completions`;
  const { data: chatInfo } = useFetchExternalChatInfo();
  const { send, answer, done, stopOutputMessage } = useSendMessageWithSse();
  const {
    derivedMessages,
    removeLatestMessage,
    addNewestAnswer,
    addNewestQuestion,
    setDerivedMessages,
    virtualizer,
    messageContainerRef,
    removeAllMessages,
    removeAllMessagesExceptFirst,
  } = useSelectDerivedMessages();
  const [hasError, setHasError] = useState(false);
  const [sessionId, setSessionId] = useState<string>();
  const [isInitializing, setIsInitializing] = useState(true);
  const initializationRequestIdRef = useRef<string>();
  const initializationControllerRef = useRef<AbortController>();

  const sendMessage = useCallback(
    async (
      message: Message,
      id?: string,
      enableThinking?: string,
      enableInternet?: boolean,
    ) => {
      if (!sessionId) return;
      const res = await send(completionUrl, {
        conversation_id: id ?? conversationId,
        quote: true,
        question: message.content,
        session_id: sessionId,
        reasoning: Number(enableThinking),
        internet: enableInternet,
        ...(chatInfo?.llm_id ? { model_name: chatInfo.llm_id } : {}),
      });

      if (isCompletionError(res)) {
        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      }
    },
    [
      send,
      completionUrl,
      conversationId,
      sessionId,
      setValue,
      removeLatestMessage,
      chatInfo,
    ],
  );

  const handleSendMessage = useCallback(
    async (
      message: Message,
      enableThinking?: string,
      enableInternet?: boolean,
    ) => {
      sendMessage(message, undefined, enableThinking, enableInternet);
    },
    [sendMessage],
  );

  const fetchSessionId = useCallback(async () => {
    initializationControllerRef.current?.abort();
    stopOutputMessage();
    const requestId = uuid();
    const controller = new AbortController();
    initializationRequestIdRef.current = requestId;
    initializationControllerRef.current = controller;

    setSessionId(undefined);
    setIsInitializing(true);
    setHasError(false);
    setDerivedMessages([]);
    const payload = { question: '' };
    const ret = await send(
      completionUrl,
      { ...payload, ...data, chatBoxId: requestId },
      controller,
    );
    if (initializationRequestIdRef.current !== requestId) return;
    if (!ret || isCompletionError(ret)) {
      message.error(ret?.data?.message ?? 'Unknown error');
      setHasError(true);
      setIsInitializing(false);
    }
  }, [send, completionUrl, data, setDerivedMessages, stopOutputMessage]);

  useEffect(() => {
    fetchSessionId();
    return () => {
      initializationRequestIdRef.current = undefined;
      initializationControllerRef.current?.abort();
    };
  }, [fetchSessionId]);

  useEffect(() => {
    if (answer.chatBoxId) {
      if (answer.chatBoxId !== initializationRequestIdRef.current) return;
      const initializedSessionId = answer.session_id;
      if (initializedSessionId) {
        setSessionId(initializedSessionId);
        setIsInitializing(false);
        setHasError(false);
      }
      if (answer.answer) {
        addNewestAnswer(answer);
      }
      return;
    }
    if (answer.answer) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer]);

  const handlePressEnter = useCallback(
    ({
      enableThinking,
      enableInternet,
    }: NextMessageInputOnPressEnterParameter) => {
      if (trim(value) === '' || !done || !sessionId) return;
      const id = uuid();
      if (done) {
        setValue('');
        addNewestQuestion({
          content: value,
          doc_ids: [],
          id,
          role: MessageType.User,
        });
        handleSendMessage(
          {
            content: value.trim(),
            id,
            role: MessageType.User,
          },
          enableThinking,
          enableInternet,
        );
      }
    },
    [addNewestQuestion, done, handleSendMessage, sessionId, setValue, value],
  );

  return {
    handlePressEnter,
    handleInputChange,
    value,
    sendLoading: isInitializing || !done,
    loading: false,
    derivedMessages,
    hasError,
    retryInitialization: fetchSessionId,
    stopOutputMessage,
    virtualizer,
    messageContainerRef,
    removeAllMessages,
    removeAllMessagesExceptFirst,
  };
};
