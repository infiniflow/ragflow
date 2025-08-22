import { useFetchRelatedQuestions } from '@/hooks/chat-hooks';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useTestChunkAllRetrieval,
  useTestChunkRetrieval,
} from '@/hooks/knowledge-hooks';
import {
  useGetPaginationWithRouter,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { IAnswer } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { get, isEmpty, isEqual, trim } from 'lodash';
import {
  ChangeEventHandler,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import {
  useGetSharedSearchParams,
  useSearchFetchMindMap,
} from '../next-search/hooks';

export const useSendQuestion = (kbIds: string[], tenantId?: string) => {
  const { sharedId } = useGetSharedSearchParams();
  const { send, answer, done, stopOutputMessage } = useSendMessageWithSse(
    sharedId ? api.askShare : api.ask,
  );

  const { testChunk, loading } = useTestChunkRetrieval(tenantId);
  const { testChunkAll } = useTestChunkAllRetrieval(tenantId);
  const [sendingLoading, setSendingLoading] = useState(false);
  const [currentAnswer, setCurrentAnswer] = useState({} as IAnswer);
  const { fetchRelatedQuestions, data: relatedQuestions } =
    useFetchRelatedQuestions(tenantId);
  const [searchStr, setSearchStr] = useState<string>('');
  const [isFirstRender, setIsFirstRender] = useState(true);
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);

  const { pagination, setPagination } = useGetPaginationWithRouter();

  const sendQuestion = useCallback(
    (question: string) => {
      const q = trim(question);
      if (isEmpty(q)) return;
      setPagination({ page: 1 });
      setIsFirstRender(false);
      setCurrentAnswer({} as IAnswer);
      setSendingLoading(true);
      send({ kb_ids: kbIds, question: q, tenantId });
      testChunk({
        kb_id: kbIds,
        highlight: true,
        question: q,
        page: 1,
        size: pagination.pageSize,
      });

      fetchRelatedQuestions(q);
    },
    [
      send,
      testChunk,
      kbIds,
      fetchRelatedQuestions,
      setPagination,
      pagination.pageSize,
      tenantId,
    ],
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

      testChunkAll({
        kb_id: kbIds,
        highlight: true,
        question: q,
        doc_ids: [],
        page,
        size,
      });
    },
    [
      searchStr,
      sendingLoading,
      testChunk,
      kbIds,
      selectedDocumentIds,
      testChunkAll,
    ],
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
    searchStr,
    setSearchStr,
    isFirstRender,
    selectedDocumentIds,
    isSearchStrEmpty: isEmpty(trim(searchStr)),
    stopOutputMessage,
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

export const useShowMindMapDrawer = (
  kbIds: string[],
  question: string,
  searchId = '',
) => {
  const { visible, showModal, hideModal } = useSetModalState();
  const ref = useRef<any>();

  const {
    fetchMindMap,
    data: mindMap,
    loading: mindMapLoading,
  } = useSearchFetchMindMap();

  const handleShowModal = useCallback(() => {
    const searchParams = { question: trim(question), kb_ids: kbIds, searchId };
    if (
      !isEmpty(searchParams.question) &&
      !isEqual(searchParams, ref.current)
    ) {
      ref.current = searchParams;
      fetchMindMap(searchParams);
    }
    showModal();
  }, [fetchMindMap, showModal, question, kbIds, searchId]);

  return {
    mindMap,
    mindMapVisible: visible,
    mindMapLoading,
    showMindMapModal: handleShowModal,
    hideMindMapModal: hideModal,
  };
};

export const usePendingMindMap = () => {
  const [count, setCount] = useState<number>(0);
  const ref = useRef<NodeJS.Timeout>();

  const setCountInterval = useCallback(() => {
    ref.current = setInterval(() => {
      setCount((pre) => {
        if (pre > 40) {
          clearInterval(ref?.current);
        }
        return pre + 1;
      });
    }, 1000);
  }, []);

  useEffect(() => {
    setCountInterval();
    return () => {
      clearInterval(ref?.current);
    };
  }, [setCountInterval]);

  return Number(((count / 43) * 100).toFixed(0));
};
