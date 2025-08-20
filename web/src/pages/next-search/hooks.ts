import message from '@/components/ui/message';
import { SharedFrom } from '@/constants/chat';
import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import {
  useGetPaginationWithRouter,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useSetPaginationParams } from '@/hooks/route-hook';
import { useKnowledgeBaseId } from '@/hooks/use-knowledge-request';
import { ResponsePostType } from '@/interfaces/database/base';
import { IAnswer } from '@/interfaces/database/chat';
import { ITestingResult } from '@/interfaces/database/knowledge';
import { IAskRequestBody } from '@/interfaces/request/chat';
import chatService from '@/services/chat-service';
import kbService from '@/services/knowledge-service';
import searchService from '@/services/search-service';
import api from '@/utils/api';
import { useMutation } from '@tanstack/react-query';
import { has, isEmpty, trim } from 'lodash';
import {
  ChangeEventHandler,
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { useSearchParams } from 'umi';
import { ISearchAppDetailProps } from '../next-searches/hooks';
import { useShowMindMapDrawer } from '../search/hooks';
import { useClickDrawer } from './document-preview-modal/hooks';

export interface ISearchingProps {
  searchText?: string;
  data: ISearchAppDetailProps;
  setIsSearching?: Dispatch<SetStateAction<boolean>>;
  setSearchText?: Dispatch<SetStateAction<string>>;
}

export type ISearchReturnProps = ReturnType<typeof useSearching>;

export const useGetSharedSearchParams = () => {
  const [searchParams] = useSearchParams();
  const data_prefix = 'data_';
  const data = Object.fromEntries(
    searchParams
      .entries()
      .filter(([key]) => key.startsWith(data_prefix))
      .map(([key, value]) => [key.replace(data_prefix, ''), value]),
  );
  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
    locale: searchParams.get('locale'),
    tenantId: searchParams.get('tenantId'),
    data: data,
    visibleAvatar: searchParams.get('visible_avatar')
      ? searchParams.get('visible_avatar') !== '1'
      : true,
  };
};

export const useSearchFetchMindMap = () => {
  const [searchParams] = useSearchParams();
  const sharedId = searchParams.get('shared_id');
  const fetchMindMapFunc = sharedId
    ? searchService.mindmapShare
    : chatService.getMindMap;
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['fetchMindMap'],
    gcTime: 0,
    mutationFn: async (params: IAskRequestBody) => {
      try {
        const ret = await fetchMindMapFunc(params);
        return ret?.data?.data ?? {};
      } catch (error: any) {
        if (has(error, 'message')) {
          message.error(error.message);
        }

        return [];
      }
    },
  });

  return { data, loading, fetchMindMap: mutateAsync };
};

export const useTestChunkRetrieval = (
  tenantId?: string,
): ResponsePostType<ITestingResult> & {
  testChunk: (...params: any[]) => void;
} => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const { page, size: pageSize } = useSetPaginationParams();
  const [searchParams] = useSearchParams();
  const shared_id = searchParams.get('shared_id');
  const retrievalTestFunc = shared_id
    ? kbService.retrievalTestShare
    : kbService.retrieval_test;
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['testChunk'], // This method is invalid
    gcTime: 0,
    mutationFn: async (values: any) => {
      const { data } = await retrievalTestFunc({
        ...values,
        kb_id: values.kb_id ?? knowledgeBaseId,
        page,
        size: pageSize,
        tenant_id: tenantId,
      });
      if (data.code === 0) {
        const res = data.data;
        return {
          ...res,
          documents: res.doc_aggs,
        };
      }
      return (
        data?.data ?? {
          chunks: [],
          documents: [],
          total: 0,
        }
      );
    },
  });

  return {
    data: data ?? { chunks: [], documents: [], total: 0 },
    loading,
    testChunk: mutateAsync,
  };
};

export const useTestChunkAllRetrieval = (
  tenantId?: string,
): ResponsePostType<ITestingResult> & {
  testChunkAll: (...params: any[]) => void;
} => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const { page, size: pageSize } = useSetPaginationParams();
  const [searchParams] = useSearchParams();
  const shared_id = searchParams.get('shared_id');
  const retrievalTestFunc = shared_id
    ? kbService.retrievalTestShare
    : kbService.retrieval_test;
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['testChunkAll'], // This method is invalid
    gcTime: 0,
    mutationFn: async (values: any) => {
      const { data } = await retrievalTestFunc({
        ...values,
        kb_id: values.kb_id ?? knowledgeBaseId,
        doc_ids: [],
        page,
        size: pageSize,
        tenant_id: tenantId,
      });
      if (data.code === 0) {
        const res = data.data;
        return {
          ...res,
          documents: res.doc_aggs,
        };
      }
      return (
        data?.data ?? {
          chunks: [],
          documents: [],
          total: 0,
        }
      );
    },
  });

  return {
    data: data ?? { chunks: [], documents: [], total: 0 },
    loading,
    testChunkAll: mutateAsync,
  };
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
export const useFetchRelatedQuestions = (
  tenantId?: string,
  searchId?: string,
) => {
  const [searchParams] = useSearchParams();
  const shared_id = searchParams.get('shared_id');
  const retrievalTestFunc = shared_id
    ? searchService.getRelatedQuestionsShare
    : chatService.getRelatedQuestions;
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['fetchRelatedQuestions'],
    gcTime: 0,
    mutationFn: async (question: string): Promise<string[]> => {
      const { data } = await retrievalTestFunc({
        question,
        tenant_id: tenantId,
        search_id: searchId,
      });

      return data?.data ?? [];
    },
  });

  return { data, loading, fetchRelatedQuestions: mutateAsync };
};

export const useSendQuestion = (
  kbIds: string[],
  tenantId?: string,
  searchId: string = '',
  related_search: boolean = false,
) => {
  const { sharedId } = useGetSharedSearchParams();
  const { send, answer, done, stopOutputMessage } = useSendMessageWithSse(
    sharedId ? api.askShare : api.ask,
  );

  const { testChunk, loading } = useTestChunkRetrieval(tenantId);
  const { testChunkAll } = useTestChunkAllRetrieval(tenantId);
  const [sendingLoading, setSendingLoading] = useState(false);
  const [currentAnswer, setCurrentAnswer] = useState({} as IAnswer);
  const { fetchRelatedQuestions, data: relatedQuestions } =
    useFetchRelatedQuestions(tenantId, searchId);
  const [searchStr, setSearchStr] = useState<string>('');
  const [isFirstRender, setIsFirstRender] = useState(true);
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);

  const { pagination, setPagination } = useGetPaginationWithRouter();

  const sendQuestion = useCallback(
    (question: string, enableAI: boolean = true) => {
      const q = trim(question);
      if (isEmpty(q)) return;
      setPagination({ page: 1 });
      setIsFirstRender(false);
      setCurrentAnswer({} as IAnswer);
      if (enableAI) {
        setSendingLoading(true);
        send({ kb_ids: kbIds, question: q, tenantId, search_id: searchId });
      }
      testChunk({
        kb_id: kbIds,
        highlight: true,
        question: q,
        page: 1,
        size: pagination.pageSize,
        search_id: searchId,
      });

      if (related_search) {
        fetchRelatedQuestions(q);
      }
    },
    [
      send,
      testChunk,
      kbIds,
      fetchRelatedQuestions,
      setPagination,
      pagination.pageSize,
      tenantId,
      searchId,
      related_search,
    ],
  );

  const handleSearchStrChange: ChangeEventHandler<HTMLInputElement> =
    useCallback((e) => {
      setSearchStr(e.target.value);
    }, []);

  const handleClickRelatedQuestion = useCallback(
    (question: string, enableAI: boolean = true) =>
      () => {
        if (sendingLoading) return;

        setSearchStr(question);
        sendQuestion(question, enableAI);
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
        search_id: searchId,
      });

      testChunkAll({
        kb_id: kbIds,
        highlight: true,
        question: q,
        doc_ids: [],
        page,
        size,
        search_id: searchId,
      });
    },
    [
      searchStr,
      sendingLoading,
      testChunk,
      kbIds,
      selectedDocumentIds,
      testChunkAll,
      searchId,
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

export const useSearching = ({
  searchText,
  data: searchData,
  setSearchText,
}: ISearchingProps) => {
  const { tenantId } = useGetSharedSearchParams();
  const {
    sendQuestion,
    handleClickRelatedQuestion,
    handleTestChunk,
    setSelectedDocumentIds,
    answer,
    sendingLoading,
    relatedQuestions,
    searchStr,
    loading,
    isFirstRender,
    selectedDocumentIds,
    isSearchStrEmpty,
    setSearchStr,
    stopOutputMessage,
  } = useSendQuestion(
    searchData.search_config.kb_ids,
    tenantId as string,
    searchData.id,
    searchData.search_config.related_search,
  );

  const handleSearchStrChange = useCallback(
    (value: string) => {
      setSearchStr(value);
    },
    [setSearchStr],
  );

  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  useEffect(() => {
    if (searchText) {
      setSearchStr(searchText);
      sendQuestion(searchText, searchData.search_config.summary);
      setSearchText?.('');
    }
  }, [
    searchText,
    sendQuestion,
    setSearchStr,
    setSearchText,
    searchData.search_config.summary,
  ]);

  const {
    mindMapVisible,
    hideMindMapModal,
    showMindMapModal,
    mindMapLoading,
    mindMap,
  } = useShowMindMapDrawer(
    searchData.search_config.kb_ids,
    searchStr,
    searchData.id,
  );
  const { chunks, total } = useSelectTestingResult();

  const handleSearch = useCallback(
    (value: string) => {
      sendQuestion(value, searchData.search_config.summary);
      setSearchStr?.(value);
      hideMindMapModal();
    },
    [
      setSearchStr,
      sendQuestion,
      hideMindMapModal,
      searchData.search_config.summary,
    ],
  );

  const { pagination, setPagination } = useGetPaginationWithRouter();
  const onChange = (pageNumber: number, pageSize: number) => {
    setPagination({ page: pageNumber, pageSize });
    handleTestChunk(selectedDocumentIds, pageNumber, pageSize);
  };

  return {
    sendQuestion,
    handleClickRelatedQuestion,
    handleSearchStrChange,
    handleTestChunk,
    setSelectedDocumentIds,
    answer,
    sendingLoading,
    relatedQuestions,
    searchStr,
    loading,
    isFirstRender,
    selectedDocumentIds,
    isSearchStrEmpty,
    setSearchStr,
    stopOutputMessage,

    visible,
    hideModal,
    documentId,
    selectedChunk,
    clickDocumentButton,
    mindMapVisible,
    hideMindMapModal,
    showMindMapModal,
    mindMapLoading,
    mindMap,
    chunks,
    total,
    handleSearch,
    pagination,
    onChange,
  };
};

export const useCheckSettings = (data: ISearchAppDetailProps) => {
  if (!data) {
    return {
      openSetting: false,
    };
  }
  const { search_config, name } = data;
  const { kb_ids } = search_config;
  return {
    openSetting: kb_ids && kb_ids.length && name ? false : true,
  };
};
