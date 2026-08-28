import { useFetchAgent } from '@/hooks/use-agent-request';
import { useCallback } from 'react';
import { useParams } from 'react-router';
import request from '@/utils/request';
import api from '@/utils/api';
import useGraphStore from '../store';
import { downloadDsl } from '../utils/download-dsl';
import { exportDsl } from '../utils/dsl-bridge';

export const useHandleExportJsonFile = () => {
  const { id } = useParams();
  const { data } = useFetchAgent();
  const { nodes, edges } = useGraphStore((state) => state);

  const handleExportJson = useCallback(async () => {
    // For cross-server portability, try to fetch portable DSL (ids -> names).
    // Fallback to local exportDsl if portable fetch fails.
    try {
      if (id) {
        const { data: res } = await request.get(api.getAgent(id), {
          params: { portable: true },
        });
        const portableDsl = (res as any)?.data?.dsl;
        if (portableDsl && typeof portableDsl === 'object') {
          downloadDsl(portableDsl, data.title);
          return;
        }
      }
    } catch {}
    const full = exportDsl(nodes, edges, data?.dsl ?? {});
    downloadDsl(full, data.title);
  }, [nodes, edges, data?.dsl, data.title, id]);

  return {
    handleExportJson,
  };
};
