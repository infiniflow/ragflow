import sonnerMessage from '@/components/ui/message';
import { MessageType } from '@/constants/chat';
import {
  useCreateAgentSession,
  useFetchAgent,
  useFetchSessionById,
} from '@/hooks/use-agent-request';
import { IAgentLogMessage } from '@/interfaces/database/agent';
import { useSendAgentMessage } from '@/pages/agent/chat/use-send-agent-message';
import api from '@/utils/api';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { isEmpty } from 'lodash';
import { useCallback, useEffect, useRef } from 'react';
import { useParams } from 'react-router';
import { useExploreUrlParams } from './use-explore-url-params';

export const useSendSessionMessage = () => {
  const { setSessionId, sessionId } = useExploreUrlParams();
  const { data: sessionData, loading: sessionLoading } =
    useFetchSessionById(sessionId);

  const { data: canvasInfo } = useFetchAgent();

  const { id: canvasId } = useParams();

  const { createAgentSession } = useCreateAgentSession();

  const isCreatingSession = useRef(false);

  const { setDerivedMessages, ...chatLogic } = useSendAgentMessage({
    url: api.runCanvasExplore(canvasId!),
  });

  useEffect(() => {
    if (sessionData?.message && sessionId) {
      const initialMessages = sessionData.message.map(
        (msg: IAgentLogMessage) => ({
          role:
            msg.role === 'assistant' ? MessageType.Assistant : MessageType.User,
          content: msg.content,
          id: buildMessageUuidWithRole({ role: msg.role, id: msg.id }),
          ...(msg.role === 'assistant' && {
            reference: sessionData.reference,
          }),
        }),
      );
      setDerivedMessages(initialMessages);
    }
  }, [sessionId, sessionData, setDerivedMessages]);

  useEffect(() => {
    return () => {
      if (!sessionId) {
        setDerivedMessages([]);
      }
    };
  }, [sessionId, setDerivedMessages]);

  const handlePressEnter = useCallback(async () => {
    if (isCreatingSession.current) {
      return;
    }

    let exploreSessionId = sessionId;

    if (isEmpty(sessionId) && canvasId) {
      isCreatingSession.current = true;
      try {
        const sessionName = chatLogic.value?.trim() || 'New Session';
        const result = await createAgentSession({
          id: canvasId,
          name: sessionName,
        });

        exploreSessionId = result.id;

        setSessionId(result.id, false);

        setTimeout(() => {
          isCreatingSession.current = false;
        }, 100);
      } catch (error) {
        isCreatingSession.current = false;
        sonnerMessage.error('Failed to create session');
        console.error('Failed to create session:', error);
        return;
      }
    }

    return chatLogic.handlePressEnter?.({ exploreSessionId });
  }, [sessionId, canvasId, chatLogic, createAgentSession, setSessionId]);

  return {
    ...chatLogic,
    handlePressEnter,
    canvasInfo,
    sessionLoading,
  };
};
