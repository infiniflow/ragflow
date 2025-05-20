import { IFlow } from '@/interfaces/database/flow';
import flowService from '@/services/flow-service';
import { useQuery } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { useCallback } from 'react';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';

export const enum AgentApiAction {
  FetchAgentList = 'fetchAgentList',
}

export const useFetchAgentListByPage = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } = useQuery<{
    kbs: IFlow[];
    total: number;
  }>({
    queryKey: [
      AgentApiAction.FetchAgentList,
      {
        debouncedSearchString,
        ...pagination,
      },
    ],
    initialData: { kbs: [], total: 0 },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await flowService.listCanvasTeam({
        keywords: debouncedSearchString,
        page_size: pagination.pageSize,
        page: pagination.current,
      });

      return data?.data ?? [];
    },
  });

  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      // setPagination({ page: 1 }); // TODO: 这里导致重复请求
      handleInputChange(e);
    },
    [handleInputChange],
  );

  return {
    data: data.kbs,
    loading,
    searchString,
    handleInputChange: onInputChange,
    pagination: { ...pagination, total: data?.total },
    setPagination,
  };
};
