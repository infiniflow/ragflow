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

import {
  DatasetSkillPage,
  DatasetSkillTree,
} from '@/interfaces/database/dataset-skill';
import datasetSkillService from '@/services/dataset-skill-service';
import { useQuery } from '@tanstack/react-query';

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
