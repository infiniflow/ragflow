import { EmptyDsl, Operator } from '@/constants/agent';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { downloadJsonFile } from '@/utils/file-util';
import { cloneDeepWith, get, isPlainObject, pick } from 'lodash';
import { useCallback } from 'react';
import { useBuildDslData } from './use-build-dsl';

/**
 * Recursively clear sensitive fields (api_key) from the DSL object
 */

const clearSensitiveFields = <T>(obj: T): T =>
  cloneDeepWith(obj, (value) => {
    if (
      isPlainObject(value) &&
      [Operator.TavilySearch, Operator.TavilyExtract, Operator.Google].includes(
        value.component_name,
      ) &&
      get(value, 'params.api_key')
    ) {
      return { ...value, params: { ...value.params, api_key: '' } };
    }
  });

export const useHandleExportJsonFile = () => {
  const { buildDslData } = useBuildDslData();
  const { data } = useFetchAgent();

  const handleExportJson = useCallback(() => {
    const dsl = pick(buildDslData(), ['graph', 'globals', 'variables']);

    const sanitizedDsl = clearSensitiveFields(dsl) as typeof dsl;

    const nextDsl = {
      ...sanitizedDsl,
      globals: { ...sanitizedDsl.globals, ...EmptyDsl.globals },
    };

    downloadJsonFile(nextDsl, `${data.title}.json`);
  }, [buildDslData, data.title]);

  return {
    handleExportJson,
  };
};
