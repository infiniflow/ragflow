import { useTestChunkRetrieval } from '@/hooks/knowledge-hooks';
import { useSendMessageWithSse } from '@/hooks/logic-hooks';
import api from '@/utils/api';
import { useCallback } from 'react';

export const useSendQuestion = (kbIds: string[]) => {
  const { send, answer, done } = useSendMessageWithSse(api.ask);
  const { testChunk, loading } = useTestChunkRetrieval();

  const sendQuestion = useCallback(
    (question: string) => {
      send({ kb_ids: kbIds, question });
      testChunk({ kb_id: kbIds, highlight: true, question });
    },
    [send, testChunk, kbIds],
  );

  return { sendQuestion, answer, loading };
};
