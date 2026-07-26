import MarkdownEditor from '@/components/markdown-editor';
import { Card } from '@/components/ui/card';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { DatasetSkillKeys } from '@/hooks/use-dataset-skill-request';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import {
  GenerateStatus,
  GenerateType,
} from '@/pages/dataset/dataset/generate-button/constants';
import { useTraceRunData } from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';
import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { useParams } from 'react-router';

import CompilationEmptyState from './empty-state';
import { useCompilationSkill } from './hooks/use-compilation-skill';
import { CompilationLoadingCard } from './loading-card';
import { SkillsLeftPanel } from './skills-left-panel';

export function SkillsView() {
  const { id } = useParams();
  const queryClient = useQueryClient();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const {
    selectedSkill,
    setSelectedSkill,
    skillTree,
    skillTreeLoading,
    skillPage,
  } = useCompilationSkill();

  const { data: skillRunData } = useTraceRunData(GenerateType.ToSkills);
  const { status: skillStatus } = useGenerateStatus(skillRunData);

  useEffect(() => {
    if (skillStatus === GenerateStatus.completed) {
      queryClient.invalidateQueries({
        queryKey: DatasetSkillKeys.tree(id!),
      });
    }
  }, [skillStatus, queryClient, id]);

  const canGenerate = (knowledgeBase?.chunk_count ?? 0) > 0;
  const isLoading = skillTreeLoading && !skillTree?.skill_with_weight?.length;
  const isEmpty = !skillTree?.skill_with_weight?.length && !skillTreeLoading;

  if (isLoading) {
    return <CompilationLoadingCard />;
  }

  if (isEmpty) {
    return (
      <CompilationEmptyState
        type="skills"
        disabled={!canGenerate}
        data={skillRunData}
      />
    );
  }

  return (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
      <ResizablePanelGroup direction="horizontal" className="flex-1">
        <ResizablePanel defaultSize={33} minSize={20} maxSize={50}>
          <SkillsLeftPanel
            selectedSkill={selectedSkill}
            onSelectSkill={setSelectedSkill}
          />
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel className="flex flex-col">
          <MarkdownEditor content={skillPage?.md_with_weight ?? ''} readOnly />
        </ResizablePanel>
      </ResizablePanelGroup>
    </Card>
  );
}
