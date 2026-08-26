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

import { useDebounce } from 'ahooks';
import { useCallback, useMemo, useState } from 'react';

export interface SearchFilterOptions<T> {
  data: T[];
  searchFields: Array<keyof T | ((item: T) => string)>;
  debounceMs?: number;
}

export function useClientSearch<T>({
  data,
  searchFields,
  debounceMs = 300,
}: SearchFilterOptions<T>) {
  const [searchKeyword, setSearchKeyword] = useState('');

  const debouncedSearchKeyword = useDebounce(searchKeyword, {
    wait: debounceMs,
  });

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      setSearchKeyword(e.target.value);
    },
    [],
  );

  const clearSearch = useCallback(() => {
    setSearchKeyword('');
  }, []);

  const filteredData = useMemo(() => {
    if (!debouncedSearchKeyword.trim()) {
      return data;
    }

    const keyword = debouncedSearchKeyword.toLowerCase().trim();

    return data.filter((item) => {
      return searchFields.some((field) => {
        let value: string;

        if (typeof field === 'function') {
          value = field(item);
        } else {
          value = String(item[field] ?? '');
        }

        return value?.toLowerCase().includes(keyword);
      });
    });
  }, [data, debouncedSearchKeyword, searchFields]);

  return {
    filteredData,
    searchKeyword,
    handleSearchChange,
    clearSearch,
    isSearching: debouncedSearchKeyword !== searchKeyword,
  };
}
