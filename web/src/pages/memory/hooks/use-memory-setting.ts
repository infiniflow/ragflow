import { useHandleSearchChange } from '@/hooks/logic-hooks';
import { IMemory } from '@/pages/memories/interface';
import memoryService from '@/services/memory-service';
import { useQuery } from '@tanstack/react-query';
import { useParams, useSearchParams } from 'react-router';
import { MemoryApiAction } from '../constant';

export const useFetchMemoryBaseConfiguration = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const memoryBaseId = searchParams.get('id') || id;
  const { handleInputChange, searchString, pagination, setPagination } =
    useHandleSearchChange();

  let queryKey: (MemoryApiAction | number)[] = [
    MemoryApiAction.FetchMemoryDetail,
  ];

  const { data, isFetching: loading } = useQuery<IMemory>({
    queryKey: [...queryKey, searchString, pagination],
    initialData: {} as IMemory,
    gcTime: 0,
    queryFn: async () => {
      if (memoryBaseId) {
        const { data } = await memoryService.getMemoryConfig(
          memoryBaseId as string,
        );
        return data?.data ?? {};
      } else {
        return {};
      }
    },
  });

  return {
    data,
    loading,
    handleInputChange,
    searchString,
    pagination,
    setPagination,
  };
};
