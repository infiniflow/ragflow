import message from '@/components/ui/message';
import { DatasetNavList } from '@/interfaces/database/dataset-nav';
import i18n from '@/locales/config';
import datasetNavService from '@/services/dataset-nav-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { useKnowledgeBaseId } from './use-knowledge-request';

export const DatasetNavKeys = {
  all: (kbId: string) => ['dataset_nav', kbId] as const,
  list: (kbId: string) => ['dataset_nav', kbId, 'list'] as const,
  children: (kbId: string, name: string) =>
    ['dataset_nav', kbId, 'children', name] as const,
};

type DatasetNavResponse<T> = {
  code?: number;
  data?: T;
  message?: string;
};

function unwrapDatasetNavResponse<T>(
  response: DatasetNavResponse<T> | undefined,
): T | null {
  if (response?.code === 0) {
    return response.data ?? null;
  }
  const errorMessage =
    (typeof response?.message === 'string' && response.message) ||
    i18n.t('datasetNav.loadFailed');
  message.error(errorMessage);
  throw new Error(errorMessage);
}

export function useFetchDatasetNav() {
  const kbId = useKnowledgeBaseId();

  const { data, isFetching: loading, isError, error } = useQuery<
    DatasetNavList | null
  >({
    queryKey: DatasetNavKeys.list(kbId),
    initialData: null,
    enabled: !!kbId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetNavService.getNav({ datasetId: kbId });
      return unwrapDatasetNavResponse<DatasetNavList>(data);
    },
  });

  return { data, loading, isError, error };
}

export function useFetchDatasetNavChildren(parentName: string | null) {
  const kbId = useKnowledgeBaseId();
  const enabled = !!kbId && !!parentName;

  const { data, isFetching: loading, isError, error } = useQuery<
    DatasetNavList | null
  >({
    queryKey: DatasetNavKeys.children(kbId, parentName ?? ''),
    initialData: null,
    enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetNavService.getNavChildren({
        datasetId: kbId,
        name: parentName!,
      });
      return unwrapDatasetNavResponse<DatasetNavList>(data);
    },
  });

  return { data, loading, isError, error };
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
