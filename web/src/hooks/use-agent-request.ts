import { IFlow } from '@/interfaces/database/flow';
import flowService from '@/services/flow-service';
import { useQuery } from '@tanstack/react-query';

export const enum AgentApiAction {
  FetchAgentList = 'fetchAgentList',
}

export const useFetchAgentList = () => {
  const { data, isFetching: loading } = useQuery<IFlow[]>({
    queryKey: [AgentApiAction.FetchAgentList],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await flowService.listCanvas();

      return data?.data ?? [];
    },
  });

  return { data, loading };
};
