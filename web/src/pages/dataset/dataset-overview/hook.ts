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

export interface IOverviewTital {
  cancelled: number;
  failed: number;
  finished: number;
  processing: number;
}
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

export interface IFileLogItem {
  create_date: string;
  create_time: number;
  document_id: string;
  document_name: string;
  document_suffix: string;
  document_type: string;
  dsl: any;
  path: string[];
  task_id: string;
  id: string;
  name: string;
  kb_id: string;
  operation_status: string;
  parser_id: string;
  pipeline_id: string;
  pipeline_title: string;
  avatar: string;
  process_begin_at: null | string;
  process_duration: number;
  progress: number;
  progress_msg: string;
  source_from: string;
  status: string;
  task_type: string;
  tenant_id: string;
  update_date: string;
  update_time: number;
}
export interface IFileLogList {
  logs: IFileLogItem[];
  total: number;
}

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
      pagination.current,
      pagination.pageSize,
      searchString,
      active,
    ],
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
    active,
    setActive,
  };
};

export { useFetchFileLogList, useFetchOverviewTital };
