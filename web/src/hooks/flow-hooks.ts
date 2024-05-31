import flowService from '@/services/flow-service';
import { useQuery } from '@tanstack/react-query';

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
