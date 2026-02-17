import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { useCallback, useState } from 'react';
import {
  FilterChange,
  FilterCollection,
  FilterType,
  FilterValue,
} from './interface';

const getFilterIds = (filter: FilterType): string[] => {
  let ids: string[] = [];
  if (!filter.list) {
    ids = [filter.id];
  }

  if (filter.list && Array.isArray(filter.list)) {
    for (const item of filter.list) {
      ids = ids.concat(getFilterIds(item));
    }
  }

  return ids;
};

const mergeFilterValue = (
  filterValue: FilterValue,
  ids: string[],
): FilterValue => {
  let value = {} as FilterValue;
  for (const key in filterValue) {
    if (Array.isArray(filterValue[key])) {
      const keyIds = filterValue[key] as string[];
      value[key] = ids.filter((id) => keyIds.includes(id));
    } else if (typeof filterValue[key] === 'object') {
      value[key] = mergeFilterValue(filterValue[key], ids);
    }
  }
  return value;
};
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

  const checkValue = useCallback((filters: FilterCollection[]) => {
    if (!filters?.length || !filterValue) {
      return;
    }
    let validFields = filters.reduce((pre, cur) => {
      return [...pre, ...getFilterIds(cur as FilterType)];
    }, [] as string[]);
    if (!validFields.length) {
      return;
    }
    setFilterValue((preValue) => {
      const newValue: FilterValue = mergeFilterValue(preValue, validFields);
      return newValue;
    });
  }, []);

  return { filterValue, setFilterValue, handleFilterSubmit, checkValue };
}
