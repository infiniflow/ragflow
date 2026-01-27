import { useFetchAgent } from '@/hooks/use-agent-request';
import { downloadJsonFile } from '@/utils/file-util';
import { pick } from 'lodash';
import { useCallback } from 'react';
import { useBuildDslData } from './use-build-dsl';

export const useHandleExportJsonFile = () => {
  const { buildDslData } = useBuildDslData();
  const { data } = useFetchAgent();

  const handleExportJson = useCallback(() => {
    const dsl = pick(buildDslData(), ['graph', 'globals', 'variables']);
    downloadJsonFile(dsl, `${data.title}.json`);
  }, [buildDslData, data.title]);

  return {
    handleExportJson,
  };
};
