import flowService from '@/services/flow-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

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
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['setFlow'],
    mutationFn: async (params: any) => {
      const { data } = await flowService.setCanvas(params);
      if (data.retcode === 0) {
        queryClient.invalidateQueries({ queryKey: ['fetchFlowList'] });
      }
      return data?.retcode;
    },
  });

  return { data, loading, setFlow: mutateAsync };
};

export const useDeleteFlow = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteFlow'],
    mutationFn: async (canvasIds: string[]) => {
      const { data } = await flowService.removeCanvas({ canvasIds });
      if (data.retcode === 0) {
        queryClient.invalidateQueries({ queryKey: ['fetchFlowList'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteFlow: mutateAsync };
};
