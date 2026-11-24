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
import { ModelResourceAccessUserIdContext } from '@/hooks/use-llm-request';
import memoryService from '@/services/memory-service';
import { useQuery } from '@tanstack/react-query';
import { useContext } from 'react';

export const enum MemoryApiAction {
  FetchMemoryList = 'fetchMemoryList',
}

export const MemoryKeys = {
  list: (accessUserId?: string) =>
    [MemoryApiAction.FetchMemoryList, accessUserId] as const,
};

export const useFetchAllMemoryList = (accessUserId?: string | null) => {
  const contextAccessUserId = useContext(ModelResourceAccessUserIdContext);
  const waitingForAccessUser =
    accessUserId === null ||
    (accessUserId === undefined && contextAccessUserId === null);
  const resourceAccessUserId =
    accessUserId === undefined
      ? (contextAccessUserId ?? undefined)
      : (accessUserId ?? undefined);
  const { data, isLoading, isError, refetch } = useQuery<IMemory[], Error>({
    queryKey: MemoryKeys.list(resourceAccessUserId),
    enabled: !waitingForAccessUser,
    queryFn: async () => {
      const { data: response } = await memoryService.getMemoryList(
        {
          params: {
            page_size: 100,
            page: 1,
            ...(resourceAccessUserId
              ? { access_user_id: resourceAccessUserId }
              : {}),
          },
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
