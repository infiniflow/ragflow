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

export const useFetchAllMemoryList = (ownerTenantId?: string) => {
  const { data, isLoading, isError, refetch } = useQuery<IMemory[], Error>({
    queryKey: [MemoryApiAction.FetchMemoryList, ownerTenantId],
    queryFn: async () => {
      // Viewing a shared canvas: fetch the canvas owner's memories instead.
      const { data: response } = await memoryService.getMemoryList(
        {
          params: ownerTenantId
            ? { page_size: 100, page: 1, tenant_id: ownerTenantId }
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
