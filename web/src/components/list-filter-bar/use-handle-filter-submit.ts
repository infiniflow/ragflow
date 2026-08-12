/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
  const value = {} as FilterValue;
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
    if (!filters?.length) {
      return;
    }

    const validFields = filters.reduce((pre, cur) => {
      return [...pre, ...getFilterIds(cur as FilterType)];
    }, [] as string[]);

    if (!validFields.length) {
      return;
    }

    setFilterValue((preValue) => {
      if (!preValue) return preValue;

      const newValue: FilterValue = mergeFilterValue(preValue, validFields);
      return newValue;
    });
  }, []);

  return { filterValue, setFilterValue, handleFilterSubmit, checkValue };
}
