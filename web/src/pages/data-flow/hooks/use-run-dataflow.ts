import { useSendMessageBySSE } from '@/hooks/use-send-message';
import api from '@/utils/api';
import { get } from 'lodash';
import { useCallback, useState } from 'react';
import { useParams } from 'umi';
import { useSaveGraphBeforeOpeningDebugDrawer } from './use-save-graph';

export function useRunDataflow(
  showLogSheet: () => void,
  hideRunOrChatDrawer: () => void,
) {
  const { send } = useSendMessageBySSE(api.runCanvas);
  const { id } = useParams();
  const [messageId, setMessageId] = useState();

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

      if (res && res?.response.status === 200 && get(res, 'data.code') === 0) {
        // fetch canvas
        hideRunOrChatDrawer();

        const msgId = get(res, 'data.data.message_id');
        if (msgId) {
          setMessageId(msgId);
        }

        return msgId;
      }
    },
    [hideRunOrChatDrawer, id, saveGraph, send],
  );

  return { run, loading: loading, messageId };
}

export type RunDataflowType = ReturnType<typeof useRunDataflow>;
