import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useCallback, useMemo } from 'react';
import { useParams, useSearchParams } from 'react-router';

export const useExploreUrlParams = () => {
  const { id: canvasId } = useParams();
  const [searchParams] = useSearchParams();
  const { navigateToAgentExplore } = useNavigatePage();

  const sessionId = useMemo(
    () => searchParams.get('sessionId') || undefined,
    [searchParams],
  );

  const isNew = useMemo(
    () => searchParams.get('isNew') || undefined,
    [searchParams],
  );

  const setCanvasId = useCallback(
    (id: string) => {
      navigateToAgentExplore(id);
    },
    [navigateToAgentExplore],
  );

  const setSessionId = useCallback(
    (id: string, isNewParam?: boolean) => {
      navigateToAgentExplore(canvasId, {
        sessionId: id,
        isNew: isNewParam,
      });
    },
    [canvasId, navigateToAgentExplore],
  );

  return {
    canvasId,
    sessionId,
    isNew,
    setCanvasId,
    setSessionId,
  };
};
