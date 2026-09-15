import { useCallback } from 'react';
import {
  useFetchArtifactAlteration,
  useRunArtifactIndex,
} from '@/hooks/use-knowledge-request';

type UseWikiUpdateOptions = {
  onUpdate?: () => void;
};

export function useWikiUpdate({ onUpdate }: UseWikiUpdateOptions = {}) {
  const { data, loading: queryLoading } = useFetchArtifactAlteration('wiki');
  const { runArtifactIndex, loading: mutationLoading } =
    useRunArtifactIndex('wiki');

  const newlyUploaded = data?.newly_uploaded ?? 0;
  const removed = data?.removed ?? 0;
  const changed = data?.changed ?? 0;
  const retryPageCount = data?.retry_page_count ?? 0;
  const hasChanges =
    newlyUploaded > 0 || removed > 0 || changed > 0 || retryPageCount > 0;

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
    changed,
    retryPageCount,
    handleUpdate,
    loading: queryLoading || mutationLoading,
  };
}
