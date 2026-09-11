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
import { isEqual } from 'lodash';
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
  const value: FilterValue = {};
  for (const key in filterValue) {
    const fieldValue = filterValue[key];
    if (Array.isArray(fieldValue)) {
      value[key] = ids.filter((id) => fieldValue.includes(id));
    } else {
      const nestedValue: Record<string, string[]> = {};
      for (const nestedKey in fieldValue) {
        nestedValue[nestedKey] = ids.filter((id) =>
          fieldValue[nestedKey].includes(id),
        );
      }
      value[key] = nestedValue;
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
      return isEqual(newValue, preValue) ? preValue : newValue;
    });
  }, []);

  return { filterValue, setFilterValue, handleFilterSubmit, checkValue };
}
