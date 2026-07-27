import { useCallback } from 'react';
import {
  useFetchArtifactAlteration,
  useRunArtifactIndex,
} from '@/hooks/use-knowledge-request';

type UseWikiUpdateOptions = {
  onUpdate?: () => void;
};

export function useWikiUpdate({ onUpdate }: UseWikiUpdateOptions = {}) {
  const { data, loading: queryLoading } = useFetchArtifactAlteration();
  const { runArtifactIndex, loading: mutationLoading } = useRunArtifactIndex();

  const newlyUploaded = data?.newly_uploaded ?? 0;
  const removed = data?.removed ?? 0;
  const hasChanges = newlyUploaded > 0 || removed > 0;

  const handleUpdate = useCallback(async () => {
    const result = await runArtifactIndex();
    if (result?.code === 0) {
      onUpdate?.();
    }
  }, [runArtifactIndex, onUpdate]);

  return {
    hasChanges,
    newlyUploaded,
    removed,
    handleUpdate,
    loading: queryLoading || mutationLoading,
  };
}
