import { IWikiCommit } from '@/interfaces/database/dataset';
import { useCallback, useState } from 'react';

type UseWikiVersionReturn = {
  selectedVersionId: string | null;
  selectedVersion: IWikiCommit | null;
  selectVersion: (version: IWikiCommit | null) => void;
  clearVersion: () => void;
};

export function useWikiVersion(): UseWikiVersionReturn {
  const [selectedVersion, setSelectedVersion] = useState<IWikiCommit | null>(
    null,
  );

  const selectVersion = useCallback((version: IWikiCommit | null) => {
    setSelectedVersion(version);
  }, []);

  const clearVersion = useCallback(() => {
    setSelectedVersion(null);
  }, []);

  return {
    selectedVersionId: selectedVersion?.id ?? null,
    selectedVersion,
    selectVersion,
    clearVersion,
  };
}
