import { GenerateType } from '@/pages/dataset/dataset/generate-button/constants';

export enum ViewMode {
  LlmWiki = 'llm-wiki',
  Skills = 'skills',
  Tree = 'tree',
  Graph = 'graph',
  MindMap = 'mindmap',
  Timeline = 'timeline',
  // SessionEssence = 'session_essence',
  // SessionGraph = 'session_graph',
}

export enum LeftPanelTab {
  Contents = 'contents',
  Graph = 'graph',
}

export const StructureKinds = [
  ViewMode.Graph,
  ViewMode.MindMap,
  ViewMode.Timeline,
  // ViewMode.SessionEssence,
  // ViewMode.SessionGraph,
] as const;

export type StructureKind = (typeof StructureKinds)[number];

export const StructureKindLabelKeyMap: Record<StructureKind, string> = {
  [ViewMode.Graph]: 'knowledgeDetails.structureGraph',
  [ViewMode.MindMap]: 'knowledgeDetails.structureMindmap',
  [ViewMode.Timeline]: 'knowledgeDetails.structureTimeline',
};

export type GenerableViewMode = Exclude<ViewMode, ViewMode.Tree>;

export const ViewModeGenerateTypeMap: Record<GenerableViewMode, GenerateType> =
  {
    [ViewMode.LlmWiki]: GenerateType.Artifact,
    [ViewMode.Skills]: GenerateType.ToSkills,
    [ViewMode.Graph]: GenerateType.KnowledgeGraph,
    [ViewMode.MindMap]: GenerateType.MindMap,
    [ViewMode.Timeline]: GenerateType.Timeline,
    // [ViewMode.SessionEssence]: GenerateType.SessionEssence,
    // [ViewMode.SessionGraph]: GenerateType.SessionGraph,
  };
