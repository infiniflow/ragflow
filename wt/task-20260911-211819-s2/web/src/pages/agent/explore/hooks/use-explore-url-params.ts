import { useCallback, useMemo } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router';

export const useExploreUrlParams = () => {
  const { id: canvasId } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

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
      navigate(`/agent/${id}/explore`);
    },
    [navigate],
  );

  const setSessionId = useCallback(
    (id: string, isNewParam?: boolean) => {
      const params = new URLSearchParams();
      if (id) params.set('sessionId', id);
      if (isNewParam) params.set('isNew', 'true');
      navigate(
        `/agent/${canvasId}/explore${params.toString() ? `?${params}` : ''}`,
      );
    },
    [canvasId, navigate],
  );

  return {
    canvasId,
    sessionId,
    isNew,
    setCanvasId,
    setSessionId,
  };
};
