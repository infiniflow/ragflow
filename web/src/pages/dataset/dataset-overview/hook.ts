import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from '@/hooks/logic-hooks';
import kbService, {
  listDataPipelineLogDocument,
  listPipelineDatasetLogs,
} from '@/services/knowledge-service';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useState } from 'react';
import { useParams, useSearchParams } from 'umi';
import { LogTabs } from './dataset-common';
import { IFileLogList, IOverviewTital } from './interface';

const useFetchOverviewTital = () => {
  const [searchParams] = useSearchParams();
  const { id } = useParams();
  const knowledgeBaseId = searchParams.get('id') || id;
  const { data } = useQuery<IOverviewTital>({
    queryKey: ['overviewTital'],
    queryFn: async () => {
      const { data: res = {} } = await kbService.getKnowledgeBasicInfo({
        kb_id: knowledgeBaseId,
      });
      return res.data || [];
    },
  });
  return { data };
};

const useFetchFileLogList = () => {
  const [searchParams] = useSearchParams();
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const { id } = useParams();
  const [active, setActive] = useState<(typeof LogTabs)[keyof typeof LogTabs]>(
    LogTabs.FILE_LOGS,
  );
  const knowledgeBaseId = searchParams.get('id') || id;
  const fetchFunc =
    active === LogTabs.DATASET_LOGS
      ? listPipelineDatasetLogs
      : listDataPipelineLogDocument;
  const { data } = useQuery<IFileLogList>({
    queryKey: [
      'fileLogList',
      knowledgeBaseId,
      pagination,
      searchString,
      active,
    ],
    placeholderData: (previousData) => {
      if (previousData === undefined) {
        return { logs: [], total: 0 };
      }
      return previousData;
    },
    enabled: true,
    queryFn: async () => {
      const { data: res = {} } = await fetchFunc({
        kb_id: knowledgeBaseId,
        page: pagination.current,
        page_size: pagination.pageSize,
        keywords: searchString,
        // order_by: '',
      });
      return res.data || [];
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
    pagination: { ...pagination, total: data?.total },
    setPagination,
    active,
    setActive,
  };
};

export { useFetchFileLogList, useFetchOverviewTital };
