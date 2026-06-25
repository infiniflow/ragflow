import { Operator } from '@/constants/agent';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { downloadJsonFile } from '@/utils/file-util';
import { cloneDeepWith, get, isPlainObject } from 'lodash';
import { useCallback } from 'react';
import useGraphStore from '../store';
import { exportDsl } from '../utils/dsl-bridge';

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
  const { data } = useFetchAgent();
  const { nodes, edges } = useGraphStore((state) => state);

  const handleExportJson = useCallback(() => {
    // bridge.exportDsl returns the canonical wire shape from current
    // graph state plus preserved DSL fields, so export can write it
    // directly after sensitive-field sanitization.
    const full = exportDsl(nodes, edges, data?.dsl ?? {});
    const sanitizedDsl = clearSensitiveFields(full);
    const nextDsl = {
      ...sanitizedDsl,
      globals: { ...(sanitizedDsl.globals ?? {}) },
    };

    downloadJsonFile(nextDsl, `${data.title}.json`);
  }, [nodes, edges, data?.dsl, data.title]);

  return {
    handleExportJson,
  };
};
