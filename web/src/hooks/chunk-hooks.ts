import { ResponseGetType } from '@/interfaces/database/base';
import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import kbService from '@/services/knowledge-service';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { PaginationProps } from 'antd';
import { useCallback, useState } from 'react';
import { useDispatch } from 'umi';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import { useGetKnowledgeSearchParams } from './route-hook';

export const useFetchChunkList = () => {
  const dispatch = useDispatch();
  const { documentId } = useGetKnowledgeSearchParams();

  const fetchChunkList = useCallback(() => {
    dispatch({
      type: 'chunkModel/chunk_list',
      payload: {
        doc_id: documentId,
      },
    });
  }, [dispatch, documentId]);

  return fetchChunkList;
};

export interface IChunkListResult {
  searchString?: string;
  handleInputChange?: React.ChangeEventHandler<HTMLInputElement>;
  pagination: PaginationProps;
  setPagination?: (pagination: { page: number; pageSize: number }) => void;
  available: number | undefined;
  handleSetAvailable: (available: number | undefined) => void;
}

export const useFetchNextChunkList = (): ResponseGetType<{
  data: IChunk[];
  total: number;
  documentInfo: IKnowledgeFile;
}> &
  IChunkListResult => {
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const { documentId } = useGetKnowledgeSearchParams();
  const { searchString, handleInputChange } = useHandleSearchChange();
  const [available, setAvailable] = useState<number | undefined>();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } = useQuery({
    queryKey: [
      'fetchChunkList',
      documentId,
      pagination.current,
      pagination.pageSize,
      debouncedSearchString,
      available,
    ],

    initialData: { data: [], total: 0, documentInfo: {} },
    // placeholderData: keepPreviousData,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.chunk_list({
        doc_id: documentId,
        page: pagination.current,
        size: pagination.pageSize,
        available_int: available,
        keywords: searchString,
      });
      if (data.retcode === 0) {
        const res = data.data;
        return {
          data: res.chunks,
          total: res.total,
          documentInfo: res.doc,
        };
      }

      return (
        data?.data ?? {
          data: [],
          total: 0,
          documentInfo: {},
        }
      );
    },
  });

  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      setPagination({ page: 1 });
      handleInputChange(e);
    },
    [handleInputChange, setPagination],
  );

  const handleSetAvailable = useCallback(
    (a: number | undefined) => {
      setPagination({ page: 1 });
      setAvailable(a);
    },
    [setAvailable, setPagination],
  );

  return {
    data,
    loading,
    pagination,
    setPagination,
    searchString,
    handleInputChange: onInputChange,
    available,
    handleSetAvailable,
  };
};

export const useSelectChunkList = () => {
  const queryClient = useQueryClient();
  const data = queryClient.getQueriesData<{
    data: IChunk[];
    total: number;
    documentInfo: IKnowledgeFile;
  }>({ queryKey: ['fetchChunkList'] });

  // console.log('🚀 ~ useSelectChunkList ~ data:', data);
  return data?.at(-1)?.[1];
};
