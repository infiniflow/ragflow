// src/pages/next-memoryes/hooks.ts

import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';
import message from '@/components/ui/message';
import { ListDeletionKey } from '@/constants/list-deletion';
import { useSetModalState } from '@/hooks/common-hooks';
import { useHandleSearchChange } from '@/hooks/logic-hooks';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import memoryService, { updateMemoryById } from '@/services/memory-service';
import { markListItemsDeleted } from '@/utils/list-deletion-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { omit } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useSearchParams } from 'react-router';
import { MemoryApiAction } from '../memory/constant';
import {
  CreateMemoryResponse,
  DeleteMemoryProps,
  DeleteMemoryResponse,
  ICreateMemoryProps,
  IMemory,
  IMemoryAppDetailProps,
  MemoryDetailResponse,
  MemoryListResponse,
  MemoryFiltersResponse,
} from './interface';

export const MemoryKeys = {
  list: () => ['memoryList'] as const,
  filters: () => ['memoryFilters'] as const,
};

export const useCreateMemory = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const {
    data,
    isError,
    isPending,
    mutateAsync: createMemoryMutation,
  } = useMutation<CreateMemoryResponse, Error, ICreateMemoryProps>({
    mutationKey: ['createMemory'],
    mutationFn: async (props) => {
      const { data: response } = await memoryService.createMemory(props);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to create memory');
      }
      queryClient.invalidateQueries({ queryKey: MemoryKeys.filters() });
      return response.data;
    },
    onSuccess: () => {
      message.success(t('message.created'));
    },
  });

  const createMemory = useCallback(
    (props: ICreateMemoryProps) => {
      return createMemoryMutation(props);
    },
    [createMemoryMutation],
  );

  return { data, isError, isPending, createMemory };
};

export const useFetchMemoryList = () => {
  const {
    handleInputChange,
    searchString,
    setSearchString,
    pagination,
    setPagination,
  } = useHandleSearchChange();
  const { filterValue, setFilterValue, handleFilterSubmit } =
    useHandleFilterSubmit();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const memoryType = Array.isArray(filterValue.memoryType)
    ? filterValue.memoryType
    : [];
  const storageType = Array.isArray(filterValue.storageType)
    ? filterValue.storageType
    : [];
  const owner = filterValue.owner;
  const requestParams: Record<string, any> = {
    keywords: debouncedSearchString,
    page_size: pagination.pageSize,
    page: pagination.current,
    memory_type: memoryType.length > 0 ? memoryType.join(',') : undefined,
    storage_type: storageType.length === 1 ? storageType[0] : undefined,
  };

  if (Array.isArray(owner) && owner.length > 0) {
    requestParams.owner_ids = owner.join(',');
  }
  const { data, isLoading, isError, refetch } = useQuery<
    MemoryListResponse,
    Error
  >({
    queryKey: [
      ...MemoryKeys.list(),
      {
        debouncedSearchString,
        ...pagination,
      },
      filterValue,
    ],
    queryFn: async () => {
      const { data: response } = await memoryService.getMemoryList(
        {
          params: requestParams,
          data: { memory_type: memoryType },
        },
        true,
      );
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to fetch memory list');
      }
      return response;
    },
  });

  // const setMemoryListParams = (newParams: MemoryListParams) => {
  //   setMemoryParams((prevParams) => ({
  //     ...prevParams,
  //     ...newParams,
  //   }));
  // };

  return {
    data,
    isLoading,
    isError,
    pagination,
    searchString,
    setSearchString,
    handleInputChange,
    setPagination,
    refetch,
    filterValue,
    setFilterValue,
    handleFilterSubmit,
  };
};

export const useFetchMemoryFilters = () => {
  const {
    data,
    isFetching: isLoading,
    isError,
  } = useQuery<MemoryFiltersResponse, Error>({
    queryKey: MemoryKeys.filters(),
    initialData: {
      filter: { owner: [], memory_type: [], storage_type: [] },
      total: 0,
    },
    queryFn: async () => {
      const { data: response } = await memoryService.getMemoryFilters(
        { params: { type: 'filter' } },
        true,
      );
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to fetch memory filters');
      }
      return response.data;
    },
  });

  return { data, isLoading, isError };
};

export const useFetchMemoryDetail = (tenantId?: string) => {
  const { id } = useParams();

  const [memoryParams] = useSearchParams();
  const shared_id = memoryParams.get('shared_id');
  const memoryId = id || shared_id;
  let param: { id: string | null; tenant_id?: string } = {
    id: memoryId,
  };
  if (shared_id) {
    param = {
      id: memoryId,
      tenant_id: tenantId,
    };
  }
  const fetchMemoryDetailFunc = shared_id
    ? memoryService.getMemoryDetailShare
    : memoryService.getMemoryDetail;

  const { data, isLoading, isError } = useQuery<MemoryDetailResponse, Error>({
    queryKey: ['memoryDetail', memoryId],
    enabled: !shared_id || !!tenantId,
    queryFn: async () => {
      const { data: response } = await fetchMemoryDetailFunc(param);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to fetch memory detail');
      }
      return response;
    },
  });

  return { data: data?.data, isLoading, isError };
};

export const useDeleteMemory = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isError,
    mutateAsync: deleteMemoryMutation,
  } = useMutation<DeleteMemoryResponse, Error, DeleteMemoryProps>({
    mutationKey: ['deleteMemory'],
    mutationFn: async (props) => {
      const { data: response } = await memoryService.deleteMemory(
        props.memory_id,
      );
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to delete memory');
      }

      queryClient.invalidateQueries({ queryKey: MemoryKeys.list() });
      queryClient.invalidateQueries({ queryKey: MemoryKeys.filters() });
      markListItemsDeleted(ListDeletionKey.MemoryList);
      return response;
    },
    onSuccess: () => {
      message.success(t('message.deleted'));
    },
    onError: (error) => {
      message.error(t('message.error', { error: error.message }));
    },
  });

  const deleteMemory = useCallback(
    (props: DeleteMemoryProps) => {
      return deleteMemoryMutation(props);
    },
    [deleteMemoryMutation],
  );

  return { data, isError, deleteMemory };
};

export const useUpdateMemory = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isError,
    isPending,
    mutateAsync: updateMemoryMutation,
  } = useMutation<any, Error, IMemoryAppDetailProps>({
    mutationKey: ['updateMemory'],
    mutationFn: async (formData) => {
      const param = omit(formData, ['id']);
      const { data: response } = await updateMemoryById(formData.id, param);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to update memory');
      }

      return response.data;
    },
    onSuccess: (data, variables) => {
      message.success(t('message.updated'));
      queryClient.invalidateQueries({
        queryKey: ['memoryDetail', variables.id],
      });
      queryClient.invalidateQueries({
        queryKey: [MemoryApiAction.FetchMemoryDetail],
      });
      queryClient.invalidateQueries({ queryKey: MemoryKeys.filters() });
    },
  });

  const updateMemory = useCallback(
    (formData: IMemoryAppDetailProps) => {
      return updateMemoryMutation(formData);
    },
    [updateMemoryMutation],
  );

  return { data, isError, isPending, updateMemory };
};

export const useRenameMemory = () => {
  const [memory, setMemory] = useState<IMemory>({} as IMemory);
  const {
    visible: openCreateModal,
    hideModal: hideChatRenameModal,
    showModal: showChatRenameModal,
  } = useSetModalState();
  const { isPending: createPending, createMemory } = useCreateMemory();
  const { isPending: updatePending, updateMemory } = useUpdateMemory();
  const memoryRenameLoading = createPending || updatePending;
  const defaultModelDictionary = useFetchDefaultModelDictionary();

  const handleShowChatRenameModal = useCallback(
    (record?: IMemory) => {
      if (record) {
        const embd_id = record.embd_id || defaultModelDictionary?.embd_id;
        const llm_id = record.llm_id || defaultModelDictionary?.llm_id;
        setMemory({
          ...record,
          embd_id,
          llm_id,
        });
      }
      showChatRenameModal();
    },
    [showChatRenameModal, defaultModelDictionary],
  );

  const handleHideModal = useCallback(() => {
    hideChatRenameModal();
    setMemory({} as IMemory);
  }, [hideChatRenameModal]);

  const onMemoryRenameOk = useCallback(
    async (data: ICreateMemoryProps, callBack?: () => void) => {
      if (memory?.id) {
        try {
          await updateMemory({
            // ...memoryDataTemp,
            name: data.name,
            id: memory?.id,
          } as unknown as IMemoryAppDetailProps);
        } catch (e) {
          console.error('error', e);
        }
      } else {
        await createMemory(data);
      }
      // if (res && !memory?.id) {
      //   navigateToMemory(res?.id)();
      // }
      callBack?.();
      handleHideModal();
    },
    [memory, createMemory, handleHideModal, updateMemory],
  );
  return {
    memoryRenameLoading,
    initialMemory: memory,
    onMemoryRenameOk,
    openCreateModal,
    hideMemoryModal: handleHideModal,
    showMemoryRenameModal: handleShowChatRenameModal,
  };
};

/**
 * Build the filter facet collections from the server-side aggregation query.
 *
 * @param filterData - Server-side filter aggregations for visible memories.
 * @returns The filter collections consumed by ListFilterBar.
 *
 * @example
 * const { data: list } = useFetchMemoryList();
 * const { filters } = useSelectFilters(list?.data?.memory_list ?? []);
 */
export function useSelectFilters(filterData: MemoryFiltersResponse) {
  const { t } = useTranslation();

  const filters: FilterCollection[] = useMemo(() => {
    return [
      {
        field: 'owner',
        list: filterData.filter.owner,
        label: t('common.owner'),
      },
      {
        field: 'memoryType',
        list: filterData.filter.memory_type,
        label: t('memories.memoryType'),
      },
      {
        field: 'storageType',
        list: filterData.filter.storage_type,
        label: t('memory.config.storageType'),
      },
    ];
  }, [filterData, t]);

  return { filters };
}
