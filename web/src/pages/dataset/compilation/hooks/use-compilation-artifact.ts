import { IArtifact } from '@/interfaces/database/dataset';
import { useCallback, useState } from 'react';
import { useWikiVersion } from './use-wiki-version';

export function useCompilationArtifact() {
  const [selectedArtifact, setSelectedArtifact] = useState<IArtifact | null>(
    null,
  );
  const { selectedVersion, selectVersion, clearVersion } = useWikiVersion();

  const handleSelectArtifact = useCallback(
    (artifact: IArtifact) => {
      setSelectedArtifact(artifact);
      clearVersion();
    },
    [clearVersion],
  );

  const clearSelectedArtifact = useCallback(() => {
    setSelectedArtifact(null);
    clearVersion();
  }, [clearVersion]);

  return {
    selectedArtifact,
    selectedVersion,
    selectVersion,
    clearVersion,
    handleSelectArtifact,
    clearSelectedArtifact,
  };
}
