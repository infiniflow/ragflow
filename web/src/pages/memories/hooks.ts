// src/pages/next-memoryes/hooks.ts

import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';
import message from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import { useHandleSearchChange } from '@/hooks/logic-hooks';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import memoryService, { updateMemoryById } from '@/services/memory-service';
import {
  buildOwnersFilter,
  groupListByArray,
  groupListByType,
} from '@/utils/list-filter-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { omit } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useSearchParams } from 'react-router';
import {
  CreateMemoryResponse,
  DeleteMemoryProps,
  DeleteMemoryResponse,
  ICreateMemoryProps,
  IMemory,
  IMemoryAppDetailProps,
  MemoryDetailResponse,
  MemoryListResponse,
} from './interface';

export const useCreateMemory = () => {
  const { t } = useTranslation();

  const createMemory = useCallback(
    async (props: ICreateMemoryProps): Promise<CreateMemoryResponse> => {
      const { data: response } = await memoryService.createMemory(props);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to create memory');
      }
      if (response.code === 0) {
        message.success(t('message.created'));
      }
      return response.data;
    },
    [t],
  );

  return { createMemory };
};

export const useFetchMemoryList = () => {
  const { handleInputChange, searchString, pagination, setPagination } =
    useHandleSearchChange();
  const { filterValue, handleFilterSubmit } = useHandleFilterSubmit();
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
      'memoryList',
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
      console.log(response);
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
    handleInputChange,
    setPagination,
    refetch,
    filterValue,
    handleFilterSubmit,
  };
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

      queryClient.invalidateQueries({ queryKey: ['memoryList'] });
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
    },
    onError: (error) => {
      message.error(t('message.error', { error: error.message }));
    },
  });

  const updateMemory = useCallback(
    (formData: IMemoryAppDetailProps) => {
      return updateMemoryMutation(formData);
    },
    [updateMemoryMutation],
  );

  return { data, isError, updateMemory };
};

export const useRenameMemory = () => {
  const [memory, setMemory] = useState<IMemory>({} as IMemory);
  const {
    visible: openCreateModal,
    hideModal: hideChatRenameModal,
    showModal: showChatRenameModal,
  } = useSetModalState();
  const { updateMemory } = useUpdateMemory();
  const { createMemory } = useCreateMemory();
  const [loading, setLoading] = useState(false);
  const { data: tenantInfo } = useFetchTenantInfo();

  const handleShowChatRenameModal = useCallback(
    (record?: IMemory) => {
      if (record) {
        const embd_id = record.embd_id || tenantInfo?.embd_id;
        const llm_id = record.llm_id || tenantInfo?.llm_id;
        setMemory({
          ...record,
          embd_id,
          llm_id,
        });
      }
      showChatRenameModal();
    },
    [showChatRenameModal, tenantInfo],
  );

  const handleHideModal = useCallback(() => {
    hideChatRenameModal();
    setMemory({} as IMemory);
  }, [hideChatRenameModal]);

  const onMemoryRenameOk = useCallback(
    async (data: ICreateMemoryProps, callBack?: () => void) => {
      // let res;
      setLoading(true);
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
      setLoading(false);
      handleHideModal();
    },
    [memory, createMemory, handleHideModal, updateMemory],
  );
  return {
    memoryRenameLoading: loading,
    initialMemory: memory,
    onMemoryRenameOk,
    openCreateModal,
    hideMemoryModal: handleHideModal,
    showMemoryRenameModal: handleShowChatRenameModal,
  };
};

export function useSelectFilters() {
  const { data: res } = useFetchMemoryList();
  const data = res?.data;

  const memoryType = useMemo(() => {
    return groupListByArray(data?.memory_list ?? [], 'memory_type');
  }, [data?.memory_list]);
  const storageType = useMemo(() => {
    return groupListByType(
      data?.memory_list ?? [],
      'storage_type',
      'storage_type',
    );
  }, [data?.memory_list]);

  const filters: FilterCollection[] = [
    buildOwnersFilter(data?.memory_list ?? [], 'owner_name'),
    {
      field: 'memoryType',
      list: memoryType,
      label: 'Memory Type',
    },
    {
      field: 'storageType',
      list: storageType,
      label: 'Storage Type',
    },
  ];

  return { filters };
}
