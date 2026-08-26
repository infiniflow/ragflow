/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { DynamicFormRef, FormFieldConfig } from '@/components/dynamic-form';
import message from '@/components/ui/message';
import { RunningStatus } from '@/constants/knowledge';
import { useSetModalState } from '@/hooks/common-hooks';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import dataSourceService, {
  dataSourceRebuild,
  dataSourceUpdate,
  deleteDataSource,
  featchDataSourceDetail,
  getDataSourceLogs,
  testDataSource,
} from '@/services/data-source-service';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { t } from 'i18next';
import { RefObject, useCallback, useMemo, useState } from 'react';
import { useParams, useSearchParams } from 'react-router';
import { DataSourceKey, useDataSourceInfo } from './constant';
import {
  IDataSorceInfo,
  IDataSource,
  IDataSourceBase,
  IDataSourceLog,
} from './interface';

const DataSourceKeys = {
  list: () => ['data-source'] as const,
  detail: (id?: string | null) => ['data-source-detail', id] as const,
  logs: (
    id: string | null,
    pagination: ReturnType<typeof useGetPaginationWithRouter>['pagination'],
    autoRefresh: boolean,
  ) => ['data-source-logs', id, pagination, autoRefresh] as const,
  logsPrefix: (id?: string | null) => ['data-source-logs', id] as const,
};

export const useListDataSource = () => {
  const { dataSourceInfo } = useDataSourceInfo();
  const { data: list, isFetching } = useQuery<IDataSource[]>({
    queryKey: DataSourceKeys.list(),
    queryFn: async () => {
      const { data } = await dataSourceService.dataSourceList();
      return data.data;
    },
  });

  const categorizeDataBySource = (data: IDataSourceBase[]) => {
    const categorizedData: Partial<Record<DataSourceKey, IDataSourceBase[]>> =
      {};

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
    const sourceList: Array<IDataSorceInfo & { list: Array<IDataSourceBase> }> =
      [];
    Object.keys(categorizedData).forEach((key: string) => {
      const k = key as DataSourceKey;
      if (dataSourceInfo[k]) {
        sourceList.push({
          id: k,
          name: dataSourceInfo[k].name,
          description: dataSourceInfo[k].description,
          icon: dataSourceInfo[k].icon,
          list: categorizedData[k] || [],
        });
      }
    });

    return sourceList;
  }, [dataSourceInfo, list]);

  return { list, categorizedList: updatedDataSourceTemplates, isFetching };
};

export const useAddDataSource = ({ isEdit = false }: { isEdit?: boolean }) => {
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
      try {
        const { data: res } = isEdit
          ? await dataSourceUpdate(data.id, {
              ...data,
              reschedule: true,
            })
          : await dataSourceService.dataSourceSet(data);
        if (res.code === 0) {
          if (isEdit && res.data?.id) {
            queryClient.setQueryData(
              DataSourceKeys.detail(res.data.id),
              res.data,
            );
            queryClient.invalidateQueries({
              queryKey: DataSourceKeys.detail(res.data.id),
            });
          }
          queryClient.invalidateQueries({ queryKey: DataSourceKeys.list() });
          message.success(t(`message.operated`));
          hideAddingModal();
          return true;
        }
        return false;
      } finally {
        setAddLoading(false);
      }
    },
    [hideAddingModal, isEdit, queryClient],
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

export const useLogListDataSource = (autoRefresh: boolean) => {
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const [currentQueryParameters] = useSearchParams();
  const id = currentQueryParameters.get('id');

  const { data, isFetching } = useQuery<{
    logs: IDataSourceLog[];
    total: number;
  }>({
    queryKey: DataSourceKeys.logs(id, pagination, autoRefresh),
    refetchInterval: autoRefresh ? 15 * 1000 : false,
    queryFn: async () => {
      const { data } = await getDataSourceLogs(id as string, {
        page_size: pagination.pageSize,
        page: pagination.current,
      });
      return data.data;
    },
  });
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
        queryClient.invalidateQueries({ queryKey: DataSourceKeys.list() });
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
    queryKey: DataSourceKeys.detail(id),
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

export const useUpdateDataSourceStatus = () => {
  const [currentQueryParameters] = useSearchParams();
  const id = currentQueryParameters.get('id');
  const queryClient = useQueryClient();
  const [loading, setLoading] = useState(false);
  const updateStatus = useCallback(
    async (status: RunningStatus.SCHEDULE | RunningStatus.CANCEL) => {
      if (!id) return;

      setLoading(true);
      try {
        const { data } = await dataSourceUpdate(id, {
          status,
        });
        if (data.code === 0) {
          queryClient.setQueryData(
            DataSourceKeys.detail(id),
            (previous?: IDataSource) => ({
              ...(previous || {}),
              ...(data.data || {}),
              status: data.data?.status ?? status,
            }),
          );

          await Promise.all([
            queryClient.invalidateQueries({
              queryKey: DataSourceKeys.detail(id),
            }),
            queryClient.invalidateQueries({ queryKey: DataSourceKeys.list() }),
            queryClient.invalidateQueries({
              queryKey: DataSourceKeys.logsPrefix(id),
            }),
          ]);

          message.success(t(`message.operated`));
        }
      } finally {
        setLoading(false);
      }
    },
    [id, queryClient],
  );
  return { updateStatus, loading };
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
        message.success(t(`message.operated`));
      }
    },
    [id],
  );
  return { handleRebuild };
};

export const useTestDataSource = (
  formRef: RefObject<DynamicFormRef | null>,
  connectorId?: string,
  fields: FormFieldConfig[] = [],
) => {
  const [currentQueryParameters] = useSearchParams();
  const id = currentQueryParameters.get('id');
  const [loading, setLoading] = useState(false);

  const handleTest = useCallback(async () => {
    const values = formRef.current?.getFilteredValues();
    const source = values?.source;
    const connectorID = id || values?.id || connectorId || source;
    if (!connectorID || !source) return;

    const fieldNames = fields
      .filter((field) => {
        if (field.name === 'id' || field.name === 'name') {
          return false;
        }
        return !field.shouldRender || field.shouldRender(values);
      })
      .map((field) => field.name);
    const isValid = await formRef.current?.trigger(
      fields.length > 0 ? fieldNames : undefined,
    );
    if (!isValid) return;

    setLoading(true);
    try {
      const config =
        values?.config && typeof values.config === 'object'
          ? values.config
          : {};
      const { data } = await testDataSource(connectorID, {
        source,
        config,
      });
      if (data.code === 0) {
        message.success(t('setting.dataSourceTestSuccess'));
      }
    } catch {
      // The request interceptor owns error notifications for failed requests.
    } finally {
      setLoading(false);
    }
  }, [connectorId, fields, formRef, id]);

  return { loading, handleTest };
};
