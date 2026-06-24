import {
  DatasetSkillPage,
  DatasetSkillTree,
  HasAnySkillResponse,
} from '@/interfaces/database/dataset-skill';
import datasetSkillService from '@/services/dataset-skill-service';
import { useQuery } from '@tanstack/react-query';
import { useKnowledgeBaseId } from './use-knowledge-request';

export const DatasetSkillKeys = {
  all: (kbId: string) => ['dataset_skill', kbId] as const,
  has: (kbId: string) => ['dataset_skill', kbId, 'has'] as const,
  tree: (kbId: string) => ['dataset_skill', kbId, 'tree'] as const,
  page: (kbId: string, skillKwd: string) =>
    ['dataset_skill', kbId, 'page', skillKwd] as const,
};

export function useHasAnySkill() {
  const kbId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<HasAnySkillResponse>({
    queryKey: DatasetSkillKeys.has(kbId),
    initialData: { has: false },
    enabled: !!kbId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetSkillService.hasAny({ datasetId: kbId });
      return data?.data ?? { has: false };
    },
  });

  return { data, loading };
}

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
