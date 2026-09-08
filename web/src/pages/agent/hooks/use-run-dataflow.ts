import message from '@/components/ui/message';
import { useSendMessageBySSE } from '@/hooks/use-send-message';
import i18n from '@/locales/config';
import api from '@/utils/api';
import { get } from 'lodash';
import { useCallback, useState } from 'react';
import { useParams } from 'react-router';
import { UseFetchLogReturnType } from './use-fetch-pipeline-log';
import { useSaveGraph } from './use-save-graph';

export function useRunDataflow({
  showLogSheet,
  setMessageId,
}: {
  showLogSheet: () => void;
} & Pick<UseFetchLogReturnType, 'setMessageId'>) {
  const { send } = useSendMessageBySSE(api.agentChatCompletion);
  const { id } = useParams();
  const { saveGraph, loading } = useSaveGraph();
  const [uploadedFileData, setUploadedFileData] =
    useState<Record<string, any>>();

  const run = useCallback(
    async (fileResponseData: Record<string, any>) => {
      const saveRet = await saveGraph();
      const success = saveRet?.code === 0;
      if (!success) return;

      // The Go backend runs the dataflow debug synchronously, so the new
      // message_id only arrives with the run response (seconds later).
      // Clear the previous polling key first so the log sheet renders the
      // loading state instead of replaying the previous run's log, which is
      // still within its Redis TTL and would otherwise look like the current
      // result.
      setMessageId('');
      showLogSheet();
      const res = await send({
        agent_id: id,
        query: '',
        'openai-compatible': false,
        session_id: null,
        files: [fileResponseData.file],
      });

      if (!res) {
        // send() swallowed a network/parse failure; without a new message_id
        // the sheet would stay on the previous run's log, so surface it.
        message.error(i18n.t('message.requestError'));
        return;
      }

      if (res?.response.status === 200 && get(res, 'data.code') === 0) {
        // fetch canvas
        setUploadedFileData(fileResponseData.file[0]);
        const msgId = get(res, 'data.data.message_id');
        if (msgId) {
          setMessageId(msgId);
        }

        return msgId;
      } else {
        // Even on a run failure the debug log (with the [ERROR] END marker) is
        // already written; surface its polling key so the log sheet can render
        // the failure timeline instead of leaving it stuck on an empty state.
        const msgId = get(res, 'data.data.message_id');
        if (msgId) {
          setMessageId(msgId);
        }
        message.error(get(res, 'data.message', ''));
      }
    },
    [id, saveGraph, send, setMessageId, setUploadedFileData, showLogSheet],
  );

  return { run, loading: loading, uploadedFileData };
}

export type RunDataflowType = ReturnType<typeof useRunDataflow>;
