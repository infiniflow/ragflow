import message from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import dataSourceService, {
  dataSourceRebuild,
  dataSourceResume,
  deleteDataSource,
  featchDataSourceDetail,
  getDataSourceLogs,
} from '@/services/data-source-service';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { t } from 'i18next';
import { useCallback, useMemo, useState } from 'react';
import { useParams, useSearchParams } from 'umi';
import { DataSourceInfo, DataSourceKey } from './contant';
import { IDataSorceInfo, IDataSource, IDataSourceBase } from './interface';

export const useListDataSource = () => {
  const { data: list, isFetching } = useQuery<IDataSource[]>({
    queryKey: ['data-source'],
    queryFn: async () => {
      const { data } = await dataSourceService.dataSourceList();
      return data.data;
    },
  });

  const categorizeDataBySource = (data: IDataSourceBase[]) => {
    const categorizedData: Record<DataSourceKey, any[]> = {} as Record<
      DataSourceKey,
      any[]
    >;

    data.forEach((item) => {
      const source = item.source;
      if (!categorizedData[source]) {
        categorizedData[source] = [];
      }
      categorizedData[source].push({
        ...item,
      });
    });

    return categorizedData;
  };

  const updatedDataSourceTemplates = useMemo(() => {
    const categorizedData = categorizeDataBySource(list || []);
    let sourcelist: Array<IDataSorceInfo & { list: Array<IDataSourceBase> }> =
      [];
    Object.keys(categorizedData).forEach((key: string) => {
      const k = key as DataSourceKey;
      sourcelist.push({
        id: k,
        name: DataSourceInfo[k].name,
        description: DataSourceInfo[k].description,
        icon: DataSourceInfo[k].icon,
        list: categorizedData[k] || [],
      });
    });

    console.log('🚀 ~ useListDataSource ~ sourcelist:', sourcelist);
    return sourcelist;
  }, [list]);

  return { list, categorizedList: updatedDataSourceTemplates, isFetching };
};

export const useAddDataSource = () => {
  const [addSource, setAddSource] = useState<IDataSorceInfo | undefined>(
    undefined,
  );
  const [addLoading, setAddLoading] = useState<boolean>(false);
  const {
    visible: addingModalVisible,
    hideModal: hideAddingModal,
    showModal,
  } = useSetModalState();
  const showAddingModal = useCallback(
    (data: IDataSorceInfo) => {
      setAddSource(data);
      showModal();
    },
    [showModal],
  );
  const queryClient = useQueryClient();

  const handleAddOk = useCallback(
    async (data: any) => {
      setAddLoading(true);
      const { data: res } = await dataSourceService.dataSourceSet(data);
      console.log('🚀 ~ handleAddOk ~ code:', res.code);
      if (res.code === 0) {
        queryClient.invalidateQueries({ queryKey: ['data-source'] });
        message.success(t(`message.operated`));
        hideAddingModal();
      }
      setAddLoading(false);
    },
    [hideAddingModal, queryClient],
  );

  return {
    addSource,
    addLoading,
    setAddSource,
    addingModalVisible,
    hideAddingModal,
    showAddingModal,
    handleAddOk,
  };
};

export const useLogListDataSource = (refresh_freq: number | false) => {
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const [currentQueryParameters] = useSearchParams();
  const id = currentQueryParameters.get('id');

  const { data, isFetching } = useQuery<{ logs: IDataSource[]; total: number }>(
    {
      queryKey: ['data-source-logs', id, pagination, refresh_freq],
      refetchInterval: refresh_freq ? refresh_freq * 60 * 1000 : false,
      queryFn: async () => {
        const { data } = await getDataSourceLogs(id as string, {
          page_size: pagination.pageSize,
          page: pagination.current,
        });
        return data.data;
      },
    },
  );
  return {
    data: data?.logs,
    isFetching,
    pagination: { ...pagination, total: data?.total },
    setPagination,
  };
};

export const useDeleteDataSource = () => {
  const [deleteLoading, setDeleteLoading] = useState<boolean>(false);
  const { hideModal, showModal } = useSetModalState();
  const queryClient = useQueryClient();
  const handleDelete = useCallback(
    async ({ id }: { id: string }) => {
      setDeleteLoading(true);
      const { data } = await deleteDataSource(id);
      if (data.code === 0) {
        message.success(t(`message.deleted`));
        queryClient.invalidateQueries({ queryKey: ['data-source'] });
      }
      setDeleteLoading(false);
    },
    [setDeleteLoading, queryClient],
  );
  return { deleteLoading, hideModal, showModal, handleDelete };
};

export const useFetchDataSourceDetail = () => {
  const [currentQueryParameters] = useSearchParams();
  const id = currentQueryParameters.get('id');
  const { data } = useQuery<IDataSource>({
    queryKey: ['data-source-detail', id],
    enabled: !!id,
    queryFn: async () => {
      const { data } = await featchDataSourceDetail(id as string);
      // if (data.code === 0) {

      // }
      return data.data;
    },
  });
  return { data };
};

export const useDataSourceResume = () => {
  const [currentQueryParameters] = useSearchParams();
  const id = currentQueryParameters.get('id');
  const queryClient = useQueryClient();
  const handleResume = useCallback(
    async (param: { resume: boolean }) => {
      const { data } = await dataSourceResume(id as string, param);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: ['data-source-detail', id] });
        message.success(t(`message.operated`));
      }
    },
    [id, queryClient],
  );
  return { handleResume };
};

export const useDataSourceRebuild = () => {
  const { id } = useParams();
  // const [currentQueryParameters] = useSearchParams();
  // const id = currentQueryParameters.get('id');
  const handleRebuild = useCallback(
    async (param: { source_id: string }) => {
      const { data } = await dataSourceRebuild(param.source_id as string, {
        kb_id: id as string,
      });
      if (data.code === 0) {
        // queryClient.invalidateQueries({ queryKey: ['data-source-detail', id] });
        message.success(t(`message.operated`));
      }
    },
    [id],
  );
  return { handleRebuild };
};
