import { useHandleSearchChange } from '@/hooks/logic-hooks';
import { IMemory } from '@/pages/memories/interface';
import { getMemoryDetailById } from '@/services/memory-service';
import { useQuery } from '@tanstack/react-query';
import { useParams, useSearchParams } from 'umi';
import { MemoryApiAction } from '../constant';

export const useFetchMemoryBaseConfiguration = (props?: {
  refreshCount?: number;
}) => {
  const { refreshCount } = props || {};
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const memoryBaseId = searchParams.get('id') || id;
  const { handleInputChange, searchString, pagination, setPagination } =
    useHandleSearchChange();

  let queryKey: (MemoryApiAction | number)[] = [
    MemoryApiAction.FetchMemoryDetail,
  ];
  if (typeof refreshCount === 'number') {
    queryKey = [MemoryApiAction.FetchMemoryDetail, refreshCount];
  }

  const { data, isFetching: loading } = useQuery<IMemory>({
    queryKey: [...queryKey, searchString, pagination],
    initialData: {} as IMemory,
    gcTime: 0,
    queryFn: async () => {
      if (memoryBaseId) {
        const { data } = await getMemoryDetailById(memoryBaseId as string, {
          //   filter: {
          //     agent_id: '',
          //   },
          keyword: searchString,
          page: pagination.current,
          page_size: pagination.size,
        });
        // setPagination({
        //   page: data?.page ?? 1,
        //   pageSize: data?.page_size ?? 10,
        //   total: data?.total ?? 0,
        // });
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
