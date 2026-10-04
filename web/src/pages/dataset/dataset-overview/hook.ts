import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from '@/hooks/logic-hooks';
import {
  getKnowledgeBasicInfo,
  listDataPipelineLogDocument,
} from '@/services/knowledge-service';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useState } from 'react';
import { useParams, useSearchParams } from 'react-router';
import { LogTabs } from './dataset-common';
import { IFileLogList, IOverviewTotal } from './interface';
import { DatasetOverviewKeys, hasActiveIngestionLogs } from './utils';

const PollIntervalMs = 5000;
const EmptyOverviewSummary: IOverviewTotal = {
  doc_num: 0,
  chunk_num: 0,
  token_num: 0,
  status: {
    unstart_count: 0,
    running_count: 0,
    cancel_count: 0,
    done_count: 0,
    fail_count: 0,
  },
};

const useFetchOverviewTotal = () => {
  const [searchParams] = useSearchParams();
  const { id } = useParams();
  const knowledgeBaseId = searchParams.get('id') || id;
  const { data } = useQuery<IOverviewTotal>({
    queryKey: DatasetOverviewKeys.summary(knowledgeBaseId),
    enabled: !!knowledgeBaseId,
    refetchInterval: PollIntervalMs,
    queryFn: async () => {
      const { data: res = {} } = await getKnowledgeBasicInfo(
        knowledgeBaseId || '',
      );
      return res.data || EmptyOverviewSummary;
    },
  });
  return { data };
};

const useFetchFileLogList = () => {
  const [searchParams] = useSearchParams();
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const { filterValue, setFilterValue, handleFilterSubmit } =
    useHandleFilterSubmit();
  const { id } = useParams();
  const [active, setActive] = useState<(typeof LogTabs)[keyof typeof LogTabs]>(
    LogTabs.FILE_LOGS,
  );
  const knowledgeBaseId = searchParams.get('id') || id;
  const logType = active === LogTabs.DATASET_LOGS ? 'dataset' : 'file';
  const { data } = useQuery<IFileLogList>({
    queryKey: DatasetOverviewKeys.logs(
      knowledgeBaseId,
      pagination.current,
      pagination.pageSize,
      searchString,
      active,
      filterValue,
    ),
    placeholderData: (previousData) => {
      if (previousData === undefined) {
        return { logs: [], total: 0 };
      }
      return previousData;
    },
    enabled: !!knowledgeBaseId,
    refetchInterval: (query) =>
      hasActiveIngestionLogs(query.state.data) ? PollIntervalMs : false,
    queryFn: async () => {
      const { data: res = {} } = await listDataPipelineLogDocument(
        knowledgeBaseId || '',
        {
          page: pagination.current,
          page_size: pagination.pageSize,
          keywords: searchString,
          log_type: logType,
          ...filterValue,
        },
      );
      return res.data || { logs: [], total: 0 };
    },
  });
  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      setPagination({ page: 1 });
      handleInputChange(e);
    },
    [handleInputChange, setPagination],
  );
  return {
    data,
    searchString,
    handleInputChange: onInputChange,
    pagination: { ...pagination, total: data?.total ?? 0 },
    setPagination,
    active,
    setActive,
    filterValue,
    setFilterValue,
    handleFilterSubmit,
  };
};

export { useFetchFileLogList, useFetchOverviewTotal };
