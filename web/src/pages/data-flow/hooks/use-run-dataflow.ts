import { useSendMessageBySSE } from '@/hooks/use-send-message';
import api from '@/utils/api';
import { useCallback } from 'react';
import { useParams } from 'umi';
import { useSaveGraphBeforeOpeningDebugDrawer } from './use-save-graph';

export function useRunDataflow(showLogSheet: () => void) {
  const { send } = useSendMessageBySSE(api.runCanvas);
  const { id } = useParams();

  const { handleRun: saveGraph, loading } =
    useSaveGraphBeforeOpeningDebugDrawer(showLogSheet!);

  const run = useCallback(
    async (fileResponseData: Record<string, any>) => {
      const success = await saveGraph();
      if (!success) return;
      const res = await send({
        id,
        query: '',
        session_id: null,
        files: [fileResponseData.file],
      });

      if (res && res?.response.status === 200 && res?.data?.code === 0) {
        // fetch canvas
      }
    },
    [id, saveGraph, send],
  );

  return { run, loading: loading };
}
