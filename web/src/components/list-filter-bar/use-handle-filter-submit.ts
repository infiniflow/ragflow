import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { useCallback, useState } from 'react';
import { FilterChange, FilterValue } from './interface';

export function useHandleFilterSubmit() {
  const [filterValue, setFilterValue] = useState<FilterValue>({});
  const { setPagination } = useGetPaginationWithRouter();
  const handleFilterSubmit: FilterChange = useCallback(
    (value) => {
      setFilterValue(value);
      setPagination({ page: 1 });
    },
    [setPagination],
  );

  return { filterValue, setFilterValue, handleFilterSubmit };
}
