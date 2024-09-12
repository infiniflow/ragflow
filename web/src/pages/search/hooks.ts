import { useFetchMindMap, useFetchRelatedQuestions } from '@/hooks/chat-hooks';
import { useTestChunkRetrieval } from '@/hooks/knowledge-hooks';
import {
  useGetPaginationWithRouter,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { IAnswer } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { get, isEmpty, trim } from 'lodash';
import { ChangeEventHandler, useCallback, useEffect, useState } from 'react';

export const useSendQuestion = (kbIds: string[]) => {
  const { send, answer, done } = useSendMessageWithSse(api.ask);
  const { testChunk, loading } = useTestChunkRetrieval();
  const [sendingLoading, setSendingLoading] = useState(false);
  const [currentAnswer, setCurrentAnswer] = useState({} as IAnswer);
  const { fetchRelatedQuestions, data: relatedQuestions } =
    useFetchRelatedQuestions();
  const {
    fetchMindMap,
    data: mindMap,
    loading: mindMapLoading,
  } = useFetchMindMap();
  const [searchStr, setSearchStr] = useState<string>('');
  const [isFirstRender, setIsFirstRender] = useState(true);
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);

  const { pagination } = useGetPaginationWithRouter();

  const sendQuestion = useCallback(
    (question: string) => {
      const q = trim(question);
      if (isEmpty(q)) return;
      setIsFirstRender(false);
      setCurrentAnswer({} as IAnswer);
      setSendingLoading(true);
      send({ kb_ids: kbIds, question: q });
      testChunk({ kb_id: kbIds, highlight: true, question: q });
      fetchMindMap({
        question: q,
        kb_ids: kbIds,
      });
      fetchRelatedQuestions(q);
    },
    [send, testChunk, kbIds, fetchRelatedQuestions, fetchMindMap],
  );

  const handleSearchStrChange: ChangeEventHandler<HTMLInputElement> =
    useCallback((e) => {
      setSearchStr(e.target.value);
    }, []);

  const handleClickRelatedQuestion = useCallback(
    (question: string) => () => {
      if (sendingLoading) return;

      setSearchStr(question);
      sendQuestion(question);
    },
    [sendQuestion, sendingLoading],
  );

  const handleTestChunk = useCallback(
    (documentIds: string[], page: number = 1, size: number = 10) => {
      const q = trim(searchStr);
      if (sendingLoading || isEmpty(q)) return;

      testChunk({
        kb_id: kbIds,
        highlight: true,
        question: q,
        doc_ids: documentIds ?? selectedDocumentIds,
        page,
        size,
      });
    },
    [sendingLoading, searchStr, kbIds, testChunk, selectedDocumentIds],
  );

  useEffect(() => {
    if (!isEmpty(answer)) {
      setCurrentAnswer(answer);
    }
    return () => {
      setCurrentAnswer({} as IAnswer);
    };
  }, [answer]);

  useEffect(() => {
    if (done) {
      setSendingLoading(false);
    }
  }, [done]);

  return {
    sendQuestion,
    handleSearchStrChange,
    handleClickRelatedQuestion,
    handleTestChunk,
    setSelectedDocumentIds,
    loading,
    sendingLoading,
    answer: currentAnswer,
    relatedQuestions: relatedQuestions?.slice(0, 5) ?? [],
    mindMap,
    mindMapLoading,
    searchStr,
    isFirstRender,
    selectedDocumentIds,
  };
};

export const useFetchBackgroundImage = () => {
  const [imgUrl, setImgUrl] = useState<string>('');

  const fetchImage = useCallback(async () => {
    try {
      const res = await fetch(
        '/HPImageArchive.aspx?format=js&idx=0&n=1&mkt=zh-CN',
      );
      const ret = await res.json();
      const url = get(ret, 'images.0.url');
      if (url) {
        setImgUrl(url);
      }
    } catch (error) {
      console.log('🚀 ~ fetchImage ~ error:', error);
    }
  }, []);

  useEffect(() => {
    fetchImage();
  }, [fetchImage]);

  return `https://cn.bing.com${imgUrl}`;
};

export const useTestRetrieval = (
  kbIds: string[],
  searchStr: string,
  sendingLoading: boolean,
) => {
  const { testChunk, loading } = useTestChunkRetrieval();
  const { pagination } = useGetPaginationWithRouter();

  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);

  const handleTestChunk = useCallback(() => {
    const q = trim(searchStr);
    if (sendingLoading || isEmpty(q)) return;

    testChunk({
      kb_id: kbIds,
      highlight: true,
      question: q,
      doc_ids: Array.isArray(selectedDocumentIds) ? selectedDocumentIds : [],
      page: pagination.current,
      size: pagination.pageSize,
    });
  }, [
    sendingLoading,
    searchStr,
    kbIds,
    testChunk,
    selectedDocumentIds,
    pagination,
  ]);

  useEffect(() => {
    handleTestChunk();
  }, [handleTestChunk]);

  return {
    loading,
    selectedDocumentIds,
    setSelectedDocumentIds,
  };
};
