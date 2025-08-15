import message from '@/components/ui/message';
import { SharedFrom } from '@/constants/chat';
import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { IAskRequestBody } from '@/interfaces/request/chat';
import chatService from '@/services/chat-service';
import searchService from '@/services/search-service';
import { useMutation } from '@tanstack/react-query';
import { has } from 'lodash';
import { Dispatch, SetStateAction, useCallback, useEffect } from 'react';
import { useSearchParams } from 'umi';
import { ISearchAppDetailProps } from '../next-searches/hooks';
import { useSendQuestion, useShowMindMapDrawer } from '../search/hooks';
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
  } = useSendQuestion(searchData.search_config.kb_ids, tenantId as string);

  const handleSearchStrChange = useCallback(
    (value: string) => {
      console.log('handleSearchStrChange', value);
      setSearchStr(value);
    },
    [setSearchStr],
  );

  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  useEffect(() => {
    if (searchText) {
      setSearchStr(searchText);
      sendQuestion(searchText);
      setSearchText?.('');
    }
  }, [searchText, sendQuestion, setSearchStr, setSearchText]);

  const {
    mindMapVisible,
    hideMindMapModal,
    showMindMapModal,
    mindMapLoading,
    mindMap,
  } = useShowMindMapDrawer(searchData.search_config.kb_ids, searchStr);
  const { chunks, total } = useSelectTestingResult();

  const handleSearch = useCallback(
    (value: string) => {
      sendQuestion(value);
      setSearchStr?.(value);
    },
    [setSearchStr, sendQuestion],
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
