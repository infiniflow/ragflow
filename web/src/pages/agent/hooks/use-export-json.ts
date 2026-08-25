import { useFetchAgent } from '@/hooks/use-agent-request';
import { useCallback } from 'react';
import useGraphStore from '../store';
import { downloadDsl } from '../utils/download-dsl';
import { exportDsl } from '../utils/dsl-bridge';

export const useHandleExportJsonFile = () => {
  const { data } = useFetchAgent();
  const { nodes, edges } = useGraphStore((state) => state);

  const handleExportJson = useCallback(() => {
    // exportDsl returns the canonical wire shape from current graph
    // state plus preserved DSL fields; downloadDsl sanitizes
    // sensitive fields and writes the file.
    const full = exportDsl(nodes, edges, data?.dsl ?? {});
    downloadDsl(full, data.title);
  }, [nodes, edges, data?.dsl, data.title]);

  return {
    handleExportJson,
  };
};
