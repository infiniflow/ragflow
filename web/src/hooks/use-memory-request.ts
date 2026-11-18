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

import { IMemory } from '@/interfaces/database/memory';
import memoryService from '@/services/memory-service';
import { useQuery } from '@tanstack/react-query';

export const enum MemoryApiAction {
  FetchMemoryList = 'fetchMemoryList',
}

export const MemoryKeys = {
  list: (canvasID?: string) =>
    [MemoryApiAction.FetchMemoryList, canvasID] as const,
};

export const useFetchAllMemoryList = (canvasID?: string) => {
  const { data, isLoading, isError, refetch } = useQuery<IMemory[], Error>({
    queryKey: MemoryKeys.list(canvasID),
    queryFn: async () => {
      const { data: response } = await memoryService.getMemoryList(
        {
          // The backend filters this list to resources both canvas owner and
          // current user can use.
          params: canvasID
            ? { page_size: 100, page: 1, canvas_id: canvasID }
            : { page_size: 100, page: 1 },
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
