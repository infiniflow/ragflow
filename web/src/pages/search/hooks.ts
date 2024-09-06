import { useTestChunkRetrieval } from '@/hooks/knowledge-hooks';
import { useSendMessageWithSse } from '@/hooks/logic-hooks';
import { IAnswer } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { isEmpty } from 'lodash';
import { useCallback, useEffect, useState } from 'react';

export const useSendQuestion = (kbIds: string[]) => {
  const { send, answer, done } = useSendMessageWithSse(api.ask);
  const { testChunk, loading } = useTestChunkRetrieval();
  const [sendingLoading, setSendingLoading] = useState(false);
  const [currentAnswer, setCurrentAnswer] = useState({} as IAnswer);

  const sendQuestion = useCallback(
    (question: string) => {
      setCurrentAnswer({} as IAnswer);
      setSendingLoading(true);
      send({ kb_ids: kbIds, question });
      testChunk({ kb_id: kbIds, highlight: true, question });
    },
    [send, testChunk, kbIds],
  );

  useEffect(() => {
    if (!isEmpty(answer)) {
      setCurrentAnswer(answer);
    }
  }, [answer]);

  useEffect(() => {
    if (done) {
      setSendingLoading(false);
    }
  }, [done]);

  return { sendQuestion, loading, sendingLoading, answer: currentAnswer };
};
