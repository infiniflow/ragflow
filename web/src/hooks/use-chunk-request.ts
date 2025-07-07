import message from '@/components/ui/message';
import { ResponseGetType } from '@/interfaces/database/base';
import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import kbService from '@/services/knowledge-service';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IChunkListResult } from './chunk-hooks';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import { useGetKnowledgeSearchParams } from './route-hook';

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
    placeholderData: (previousData: any) =>
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

export const useSwitchChunk = () => {
  const { t } = useTranslation();
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
      }
      return data?.code;
    },
  });

  return { data, loading, switchChunk: mutateAsync };
};
