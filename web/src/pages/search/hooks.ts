import { MessageType } from '@/constants/chat';
import { useTestChunkRetrieval } from '@/hooks/knowledge-hooks';
import { useSendMessageWithSse } from '@/hooks/logic-hooks';
import api from '@/utils/api';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { IMessage } from '../chat/interface';

export const useSendQuestion = (kbIds: string[]) => {
  const { send, answer, done } = useSendMessageWithSse(api.ask);
  const { testChunk, loading } = useTestChunkRetrieval();
  const [sendingLoading, setSendingLoading] = useState(false);

  const message: IMessage = useMemo(() => {
    return {
      id: '',
      content: answer.answer,
      role: MessageType.Assistant,
      reference: answer.reference,
    };
  }, [answer]);

  const sendQuestion = useCallback(
    (question: string) => {
      setSendingLoading(true);
      send({ kb_ids: kbIds, question });
      testChunk({ kb_id: kbIds, highlight: true, question });
    },
    [send, testChunk, kbIds],
  );

  useEffect(() => {
    if (done) {
      setSendingLoading(false);
    }
  }, [done]);

  return { sendQuestion, message, loading, sendingLoading };
};
