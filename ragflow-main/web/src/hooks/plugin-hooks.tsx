import { ILLMTools } from '@/interfaces/database/plugin';
import pluginService from '@/services/plugin-service';
import { useQuery } from '@tanstack/react-query';

export const useLlmToolsList = (): ILLMTools => {
  const { data } = useQuery({
    queryKey: ['llmTools'],
    initialData: [],
    queryFn: async () => {
      const { data } = await pluginService.getLlmTools();

      return data?.data ?? [];
    },
  });

  return data;
};
