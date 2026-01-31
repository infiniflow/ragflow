import { useFetchAgent } from '@/hooks/use-agent-request';
import { downloadJsonFile } from '@/utils/file-util';
import { useCallback } from 'react';
import { useBuildDslData } from './use-build-dsl';

export const useHandleExportJsonFile = () => {
  const { buildDslData } = useBuildDslData();
  const { data } = useFetchAgent();

  const handleExportJson = useCallback(() => {
    downloadJsonFile(buildDslData().graph, `${data.title}.json`);
  }, [buildDslData, data.title]);

  return {
    handleExportJson,
  };
};
