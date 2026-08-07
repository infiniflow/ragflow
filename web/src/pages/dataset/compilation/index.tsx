import BackButton from '@/components/back-button';
import {
  SelectWithSearch,
  type SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { StructureKinds, ViewMode, ViewModeLabelKeyMap } from './constants';
import { DatasetStructureView } from './dataset-structure-view';
import { LlmWikiView } from './llm-wiki-view';
import { NavTreeView } from './nav-tree-view';
import { SkillsView } from './skills-view';

export default function Compilation() {
  const { t } = useTranslation();
  const { id } = useParams();
  const { navigateToDataFile } = useNavigatePage();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.LlmWiki);

  const viewOptions = useMemo<SelectWithSearchFlagOptionType[]>(() => {
    return Object.values(ViewMode).map((mode) => ({
      value: mode,
      label: t(ViewModeLabelKeyMap[mode]),
    }));
  }, [t]);

  const handleViewModeChange = useCallback((value: string) => {
    setViewMode(value as ViewMode);
  }, []);

  const structureKind = StructureKinds.find((kind) => kind === viewMode);

  return (
    <section className="flex flex-col p-4 gap-4 h-full">
      <header className="space-y-5">
        <BackButton onClick={navigateToDataFile(id!)}>
          {t('common.back')}
        </BackButton>

        <section className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <RAGFlowAvatar
              avatar={knowledgeBase?.avatar}
              name={knowledgeBase?.name}
              className="size-10 rounded-lg"
            />
            <h2 className="text-xl font-medium text-text-primary">
              {knowledgeBase?.name}
              {t('knowledgeDetails.compilationTitleSuffix')}
            </h2>
          </div>

          <SelectWithSearch
            options={viewOptions}
            value={viewMode}
            onChange={handleViewModeChange}
            triggerClassName="w-96"
          />
        </section>
      </header>

      {viewMode === ViewMode.LlmWiki && <LlmWikiView />}
      {viewMode === ViewMode.Skills && <SkillsView />}
      {viewMode === ViewMode.Tree && <NavTreeView />}
      {structureKind && <DatasetStructureView kind={structureKind} />}
    </section>
  );
}
