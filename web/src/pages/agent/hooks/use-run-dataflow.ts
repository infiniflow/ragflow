import message from '@/components/ui/message';
import { useSendMessageBySSE } from '@/hooks/use-send-message';
import api from '@/utils/api';
import { get } from 'lodash';
import { useCallback, useState } from 'react';
import { useParams } from 'umi';
import { UseFetchLogReturnType } from './use-fetch-pipeline-log';
import { useSaveGraph } from './use-save-graph';

export function useRunDataflow({
  showLogSheet,
  setMessageId,
}: {
  showLogSheet: () => void;
} & Pick<UseFetchLogReturnType, 'setMessageId'>) {
  const { send } = useSendMessageBySSE(api.runCanvas);
  const { id } = useParams();
  const { saveGraph, loading } = useSaveGraph();
  const [uploadedFileData, setUploadedFileData] =
    useState<Record<string, any>>();

  const run = useCallback(
    async (fileResponseData: Record<string, any>) => {
      const saveRet = await saveGraph();
      const success = saveRet?.code === 0;
      if (!success) return;

      showLogSheet();
      const res = await send({
        id,
        query: '',
        session_id: null,
        files: [fileResponseData.file],
      });

      if (res && res?.response.status === 200 && get(res, 'data.code') === 0) {
        // fetch canvas
        setUploadedFileData(fileResponseData.file);
        const msgId = get(res, 'data.data.message_id');
        if (msgId) {
          setMessageId(msgId);
        }

        return msgId;
      } else {
        message.error(get(res, 'data.message', ''));
      }
    },
    [id, saveGraph, send, setMessageId, setUploadedFileData, showLogSheet],
  );

  return { run, loading: loading, uploadedFileData };
}

export type RunDataflowType = ReturnType<typeof useRunDataflow>;
