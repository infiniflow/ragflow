import { DatasetSkillKeys } from '@/hooks/use-dataset-skill-request';
import { artifactKeys } from '@/hooks/use-knowledge-request';
import { useTraceGenerate } from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router';
import { LeftPanelTab, ViewMode } from '../constants';

export function useCompilationView() {
  const { id } = useParams();
  const queryClient = useQueryClient();
  const [leftTab, setLeftTab] = useState<LeftPanelTab>(LeftPanelTab.Contents);
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.LlmWiki);

  const { artifactRunData, skillRunData } = useTraceGenerate({ open: true });
  const { status: artifactStatus } = useGenerateStatus(artifactRunData);
  const { status: skillStatus } = useGenerateStatus(skillRunData);

  useEffect(() => {
    if (viewMode === ViewMode.LlmWiki && artifactStatus === 'completed') {
      queryClient.invalidateQueries({
        queryKey: artifactKeys.listByDataset(id!),
      });
    }
  }, [viewMode, artifactStatus, queryClient, id]);

  useEffect(() => {
    if (viewMode === ViewMode.Skills && skillStatus === 'completed') {
      queryClient.invalidateQueries({
        queryKey: DatasetSkillKeys.tree(id!),
      });
    }
  }, [viewMode, skillStatus, queryClient, id]);

  const handleSwitchToGraph = useCallback(() => {
    setViewMode(ViewMode.Graph);
  }, []);

  const handleSwitchToLlmWiki = useCallback(() => {
    setViewMode(ViewMode.LlmWiki);
  }, []);

  const handleSwitchToSkills = useCallback(() => {
    setViewMode(ViewMode.Skills);
  }, []);

  const handleLeftTabChange = useCallback((value: string) => {
    setLeftTab(value as LeftPanelTab);
  }, []);

  return {
    leftTab,
    viewMode,
    artifactRunData,
    skillRunData,
    artifactStatus,
    skillStatus,
    handleSwitchToGraph,
    handleSwitchToLlmWiki,
    handleSwitchToSkills,
    handleLeftTabChange,
  };
}
