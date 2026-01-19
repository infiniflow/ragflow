import { IMemory } from '@/interfaces/database/memory';
import memoryService from '@/services/memory-service';
import { useQuery } from '@tanstack/react-query';

export const enum MemoryApiAction {
  FetchMemoryList = 'fetchMemoryList',
}

export const useFetchAllMemoryList = () => {
  const { data, isLoading, isError, refetch } = useQuery<IMemory[], Error>({
    queryKey: [MemoryApiAction.FetchMemoryList],
    queryFn: async () => {
      const { data: response } = await memoryService.getMemoryList(
        {
          params: { page_size: 100000000, page: 1 },
          data: {},
        },
        true,
      );
      return response.data.memory_list ?? [];
    },
  });

  return {
    data,
    isLoading,
    isError,
    refetch,
  };
};
