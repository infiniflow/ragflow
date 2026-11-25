import {
  ViewMode as PrivateViewMode,
  StructureKinds as PrivateStructureKinds,
  ViewModeLabelKeyMap as PrivateViewModeLabelKeyMap,
  ViewModeIconMap as PrivateViewModeIconMap,
  ProcessingTypeViewModeMap as PrivateProcessingTypeViewModeMap,
  ViewModeGenerateTypeMap as PrivateViewModeGenerateTypeMap,
} from '@/ee/pages/dataset/compilation/constants';
import { GenerateType, ProcessingType } from '@/constants/knowledge';
import {
  BookOpenText,
  ListIndentDecrease,
  ListTree,
  Waypoints,
  type LucideIcon,
} from 'lucide-react';

enum BaseViewMode {
  LlmWiki = 'llm-wiki',
  Skills = 'skills',
  Tree = 'tree',
  Graph = 'graph',
  MindMap = 'mindmap',
}

export const ViewMode = { ...BaseViewMode, ...PrivateViewMode };
export type ViewMode = (typeof ViewMode)[keyof typeof ViewMode];

export const VisibleViewModes = Object.values(ViewMode).filter(
  (mode) => mode !== ViewMode.Skills,
);

export enum LeftPanelTab {
  Contents = 'contents',
  Graph = 'graph',
}

export const StructureKinds = [
  ViewMode.Graph,
  ViewMode.MindMap,
  ...PrivateStructureKinds,
] as const;

export type StructureKind = (typeof StructureKinds)[number];

export const ViewModeLabelKeyMap: Record<ViewMode, string> = {
  [ViewMode.LlmWiki]: 'knowledgeCompilation.llmWiki',
  [ViewMode.Skills]: 'knowledgeCompilation.skills',
  [ViewMode.Tree]: 'knowledgeCompilation.navTree',
  [ViewMode.Graph]: 'knowledgeCompilation.graph',
  [ViewMode.MindMap]: 'knowledgeCompilation.structureMindmap',
  ...PrivateViewModeLabelKeyMap,
};

export type GenerableViewMode = Exclude<ViewMode, typeof ViewMode.Tree>;

export const ViewModeIconMap: Partial<Record<ViewMode, LucideIcon>> = {
  [ViewMode.LlmWiki]: BookOpenText,
  [ViewMode.Tree]: ListTree,
  [ViewMode.Graph]: Waypoints,
  [ViewMode.MindMap]: ListIndentDecrease,
  ...PrivateViewModeIconMap,
};

// Dataset-log task_type values that correspond to a compilation view.
// 'wiki'/'RAPTOR'/'GraphRAG' are legacy task types; Tree and PageIndex
// both surface in the Tree/PageIndex view.
export const ProcessingTypeViewModeMap: Record<string, ViewMode> = {
  [ProcessingType.artifact]: ViewMode.LlmWiki,
  [ProcessingType.wiki]: ViewMode.LlmWiki,
  [ProcessingType.raptor]: ViewMode.Tree,
  [ProcessingType.tree]: ViewMode.Tree,
  [ProcessingType.pageIndex]: ViewMode.Tree,
  [ProcessingType.knowledgeGraph]: ViewMode.Graph,
  GraphRAG: ViewMode.Graph,
  [ProcessingType.mindmap]: ViewMode.MindMap,
  ...PrivateProcessingTypeViewModeMap,
};

export const ViewModeGenerateTypeMap: Record<GenerableViewMode, GenerateType> =
  {
    [ViewMode.LlmWiki]: GenerateType.Artifact,
    [ViewMode.Skills]: GenerateType.ToSkills,
    [ViewMode.Graph]: GenerateType.KnowledgeGraph,
    [ViewMode.MindMap]: GenerateType.MindMap,
    ...PrivateViewModeGenerateTypeMap,
  };
