import message from '@/components/ui/message';
import {
  DatasetSkillPage,
  DatasetSkillTree,
} from '@/interfaces/database/dataset-skill';
import i18n from '@/locales/config';
import datasetSkillService from '@/services/dataset-skill-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { useKnowledgeBaseId } from './use-knowledge-request';

export const DatasetSkillKeys = {
  all: (kbId: string) => ['dataset_skill', kbId] as const,
  tree: (kbId: string) => ['dataset_skill', kbId, 'tree'] as const,
  page: (kbId: string, skillKwd: string) =>
    ['dataset_skill', kbId, 'page', skillKwd] as const,
};

export function useFetchDatasetSkillTree() {
  const kbId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<DatasetSkillTree | null>({
    queryKey: DatasetSkillKeys.tree(kbId),
    initialData: null,
    enabled: !!kbId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetSkillService.getTree({ datasetId: kbId });
      return data?.data ?? null;
    },
  });

  return { data, loading };
}

export function useFetchDatasetSkillPage(skillKwd: string | null | undefined) {
  const kbId = useKnowledgeBaseId();
  const enabled = !!kbId && !!skillKwd;

  const { data, isFetching: loading } = useQuery<DatasetSkillPage | null>({
    queryKey: DatasetSkillKeys.page(kbId, skillKwd ?? ''),
    initialData: null,
    enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetSkillService.getPage({
        datasetId: kbId,
        skillKwd: skillKwd!,
      });
      return data?.data ?? null;
    },
  });

  return { data, loading };
}

export function useDeleteDatasetSkillTree() {
  const kbId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationFn: async () => {
      const { data } = await datasetSkillService.deleteTree({
        datasetId: kbId,
      });
      if (data?.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: DatasetSkillKeys.all(kbId),
        });
      }
      return data;
    },
  });

  return { data, loading, deleteSkillTree: mutateAsync };
}

export function useDeleteDatasetSkillPage() {
  const kbId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationFn: async (skillKwd: string) => {
      const { data } = await datasetSkillService.deletePage({
        datasetId: kbId,
        skillKwd,
      });
      if (data?.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: DatasetSkillKeys.all(kbId),
        });
      }
      return data;
    },
  });

  return { data, loading, deleteSkillPage: mutateAsync };
}
