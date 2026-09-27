import {
  useFetchDatasetSkillPage,
  useFetchDatasetSkillTree,
} from '@/hooks/use-dataset-skill-request';
import { useState } from 'react';

export function useCompilationSkill() {
  const [selectedSkill, setSelectedSkill] = useState<string | null>(null);
  const { data: skillTree, loading: skillTreeLoading } =
    useFetchDatasetSkillTree();
  const { data: skillPage } = useFetchDatasetSkillPage(selectedSkill);

  return {
    selectedSkill,
    setSelectedSkill,
    skillTree,
    skillTreeLoading,
    skillPage,
  };
}
