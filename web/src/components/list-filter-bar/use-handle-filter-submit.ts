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
import { FilterChange, FilterCollection, FilterValue } from './interface';
import { pruneFilterValue } from './utils';
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
    setFilterValue((value) => pruneFilterValue(value, filters));
  }, []);

  return { filterValue, setFilterValue, handleFilterSubmit, checkValue };
}
