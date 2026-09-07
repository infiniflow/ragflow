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

import message from '@/components/ui/message';
import { DatasetNavList } from '@/interfaces/database/dataset-nav';
import i18n from '@/locales/config';
import datasetNavService from '@/services/dataset-nav-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { trim } from 'lodash';

import { useKnowledgeBaseId } from './use-knowledge-request';

export const DatasetNavKeys = {
  all: (kbId: string) => ['dataset_nav', kbId] as const,
  list: (kbId: string, keywords = '') =>
    ['dataset_nav', kbId, 'list', keywords] as const,
  children: (kbId: string, name: string) =>
    ['dataset_nav', kbId, 'children', name] as const,
};

type DatasetNavResponse<T> = {
  code?: number;
  data?: T;
  message?: string;
};

function isDatasetNavList(payload: unknown): payload is DatasetNavList {
  if (!payload || typeof payload !== 'object') {
    return false;
  }
  const candidate = payload as DatasetNavList;
  return typeof candidate.total === 'number' && Array.isArray(candidate.items);
}

function unwrapDatasetNavResponse(
  response: DatasetNavResponse<DatasetNavList> | undefined,
): DatasetNavList {
  if (response?.code !== 0 || !isDatasetNavList(response.data)) {
    const errorMessage =
      (typeof response?.message === 'string' && response.message) ||
      i18n.t('knowledgeCompilation.navLoadFailed');
    throw new Error(errorMessage);
  }
  return response.data;
}

export function useFetchDatasetNav(keywords = '') {
  const kbId = useKnowledgeBaseId();
  const trimmedKeywords = trim(keywords);

  const {
    data,
    isFetching: loading,
    isError,
    error,
    refetch,
  } = useQuery<DatasetNavList | null>({
    queryKey: DatasetNavKeys.list(kbId, trimmedKeywords),
    initialData: null,
    enabled: !!kbId,
    gcTime: 0,
    retry: false,
    queryFn: async () => {
      const { data } = await datasetNavService.getNav({
        datasetId: kbId,
        keywords: trimmedKeywords,
      });
      return unwrapDatasetNavResponse(data);
    },
  });

  return { data, loading, isError, error, refetch };
}

export function useFetchDatasetNavChildren(parentName: string | null) {
  const kbId = useKnowledgeBaseId();
  const enabled = !!kbId && !!parentName;

  const {
    data,
    isFetching: loading,
    isError,
    error,
    refetch,
  } = useQuery<DatasetNavList | null>({
    queryKey: DatasetNavKeys.children(kbId, parentName ?? ''),
    initialData: null,
    enabled,
    gcTime: 0,
    retry: false,
    queryFn: async () => {
      const { data } = await datasetNavService.getNavChildren({
        datasetId: kbId,
        name: parentName!,
      });
      return unwrapDatasetNavResponse(data);
    },
  });

  return { data, loading, isError, error, refetch };
}

export function useDeleteDatasetNav() {
  const kbId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationFn: async () => {
      const { data } = await datasetNavService.deleteNav({ datasetId: kbId });
      if (data?.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: DatasetNavKeys.all(kbId),
        });
      }
      return data;
    },
  });

  return { data, loading, deleteNav: mutateAsync };
}

export function useDeleteDatasetNavNode() {
  const kbId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationFn: async (name: string) => {
      const { data } = await datasetNavService.deleteNavNode({
        datasetId: kbId,
        name,
      });
      if (data?.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: DatasetNavKeys.all(kbId),
        });
      }
      return data;
    },
  });

  return { data, loading, deleteNavNode: mutateAsync };
}
