import { ResponseGetType, ResponseType } from '@/interfaces/database/base';
import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import kbService from '@/services/knowledge-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { PaginationProps, message } from 'antd';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import {
  useGetKnowledgeSearchParams,
  useSetPaginationParams,
} from './route-hook';

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
    placeholderData: (previousData) =>
      previousData ?? { data: [], total: 0, documentInfo: {} }, // https://github.com/TanStack/query/issues/8183
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.chunk_list({
        doc_id: documentId,
        page: pagination.current,
        size: pagination.pageSize,
        available_int: available,
        keywords: searchString,
      });
      if (data.code === 0) {
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

  return data?.at(-1)?.[1];
};

export const useDeleteChunk = () => {
  const queryClient = useQueryClient();
  const { setPaginationParams } = useSetPaginationParams();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteChunk'],
    mutationFn: async (params: { chunkIds: string[]; doc_id: string }) => {
      const { data } = await kbService.rm_chunk(params);
      if (data.code === 0) {
        setPaginationParams(1);
        queryClient.invalidateQueries({ queryKey: ['fetchChunkList'] });
      }
      return data?.code;
    },
  });

  return { data, loading, deleteChunk: mutateAsync };
};

export const useSwitchChunk = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['switchChunk'],
    mutationFn: async (params: {
      chunk_ids?: string[];
      available_int?: number;
      doc_id: string;
    }) => {
      const { data } = await kbService.switch_chunk(params);
      if (data.code === 0) {
        message.success(t('message.modified'));
        queryClient.invalidateQueries({ queryKey: ['fetchChunkList'] });
      }
      return data?.code;
    },
  });

  return { data, loading, switchChunk: mutateAsync };
};

export const useCreateChunk = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createChunk'],
    mutationFn: async (payload: any) => {
      let service = kbService.create_chunk;
      if (payload.chunk_id) {
        service = kbService.set_chunk;
      }
      const { data } = await service(payload);
      if (data.code === 0) {
        message.success(t('message.created'));
        queryClient.invalidateQueries({ queryKey: ['fetchChunkList'] });
      }
      return data?.code;
    },
  });

  return { data, loading, createChunk: mutateAsync };
};

export const useFetchChunk = (chunkId?: string): ResponseType<any> => {
  const { data } = useQuery({
    queryKey: ['fetchChunk'],
    enabled: !!chunkId,
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const data = await kbService.get_chunk({
        chunk_id: chunkId,
      });

      return data;
    },
  });

  return data;
};
