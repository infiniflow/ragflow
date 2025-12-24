import { useFetchAgent } from '@/hooks/use-agent-request';
import { downloadJsonFile } from '@/utils/file-util';
import { useCallback } from 'react';
import { useBuildDslData } from './use-build-dsl';

export const useHandleExportJsonFile = () => {
  const { buildDslData } = useBuildDslData();
  const { data } = useFetchAgent();

  const handleExportJson = useCallback(() => {
    const dslData = buildDslData();
    // Export full DSL data including conversation variables
    // Previously only graph was exported, now including variables for complete agent restoration
    downloadJsonFile(
      {
        graph: dslData.graph,
        variables: dslData.variables || {},
        components: dslData.components,
        globals: dslData.globals,
      },
      `${data.title}.json`,
    );
  }, [buildDslData, data.title]);

  return {
    handleExportJson,
  };
};
