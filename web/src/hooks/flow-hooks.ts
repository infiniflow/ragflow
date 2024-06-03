import flowService from '@/services/flow-service';
import { useMutation, useQuery } from '@tanstack/react-query';

export const useFetchFlowTemplates = () => {
  const { data } = useQuery({
    queryKey: ['fetchFlowTemplates'],
    initialData: [],
    queryFn: async () => {
      const { data } = await flowService.listTemplates();

      return data;
    },
  });

  return data;
};

export const useFetchFlowList = () => {
  const { data, isFetching: loading } = useQuery({
    queryKey: ['fetchFlowList'],
    initialData: [],
    queryFn: async () => {
      const { data } = await flowService.listCanvas();

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useSetFlow = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['fetchFlowList'],
    mutationFn: async (params: any) => {
      const { data } = await flowService.setCanvas(params);

      return data?.data ?? [];
    },
  });

  return { data, loading, setFlow: mutateAsync };
};
