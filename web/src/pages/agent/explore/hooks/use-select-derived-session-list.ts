import { useFetchSessionsByCanvasId } from '@/hooks/use-agent-request';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { useCallback, useEffect, useState } from 'react';
import { useExploreUrlParams } from './use-explore-url-params';

export const useSelectDerivedSessionList = () => {
  const [list, setList] = useState<
    Array<IAgentLogResponse & { is_new?: boolean }>
  >([]);

  const {
    data: sessions,
    total,
    loading,
    loadingMore,
    handleScroll,
  } = useFetchSessionsByCanvasId();

  const { setSessionId, isNew, canvasId } = useExploreUrlParams();

  const addTemporarySession = useCallback(() => {
    const now = Date.now() / 1000;

    const tempSession: IAgentLogResponse & { is_new?: boolean } = {
      id: '',
      message: [],
      create_date: '',
      create_time: now,
      update_date: '',
      update_time: now,
      round: 0,
      thumb_up: 0,
      errors: '',
      source: '',
      user_id: '',
      dsl: '',
      reference: [],
      is_new: true,
      name: '',
      version_title: '',
    };

    setList([tempSession, ...sessions]);

    setSessionId('', true);
  }, [sessions, setSessionId]);

  const removeTemporarySession = useCallback((sessionId: string) => {
    setList((prevList) => {
      return prevList.filter((session) => session.id !== sessionId);
    });
  }, []);

  // Sync server data to local state
  useEffect(() => {
    setList((previous) => [
      ...previous.filter((session) => session.is_new && isNew === 'true'),
      ...sessions,
    ]);
  }, [sessions, isNew, canvasId]);

  return {
    sessions: list,
    total,
    loading,
    loadingMore,
    handleScroll,
    addTemporarySession,
    removeTemporarySession,
  };
};
