import { useCallback, useState } from 'react';

import { StructureKinds, ViewMode } from './constants';
import { DatasetStructureView } from './dataset-structure-view';
import { LlmWikiView } from './llm-wiki-view';
import { NavTreeView } from './nav-tree-view';
import { SkillsView } from './skills-view';

export default function Compilation() {
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.LlmWiki);
  const [artifactsOpen, setArtifactsOpen] = useState(true);

  const handleViewModeChange = useCallback((value: ViewMode) => {
    setViewMode(value);
  }, []);

  const structureKind = StructureKinds.find((kind) => kind === viewMode);
  const viewProps = {
    viewMode,
    artifactsOpen,
    onArtifactsOpenChange: setArtifactsOpen,
    onViewModeChange: handleViewModeChange,
  };

  return (
    <section className="flex h-full min-h-0 p-4">
      {viewMode === ViewMode.LlmWiki && <LlmWikiView {...viewProps} />}
      {viewMode === ViewMode.Skills && <SkillsView />}
      {viewMode === ViewMode.Tree && <NavTreeView {...viewProps} />}
      {structureKind && (
        <DatasetStructureView kind={structureKind} {...viewProps} />
      )}
    </section>
  );
}
