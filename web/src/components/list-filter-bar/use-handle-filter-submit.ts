import { useCallback, useState } from 'react';
import { FilterChange, FilterValue } from './interface';

export function useHandleFilterSubmit() {
  const [filterValue, setFilterValue] = useState<FilterValue>({});

  const handleFilterSubmit: FilterChange = useCallback((value) => {
    setFilterValue(value);
  }, []);

  return { filterValue, setFilterValue, handleFilterSubmit };
}
