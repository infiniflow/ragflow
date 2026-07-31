import { GenerateType } from '@/constants/knowledge';

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

export const ViewModeLabelKeyMap: Record<ViewMode, string> = {
  [ViewMode.LlmWiki]: 'knowledgeDetails.llmWiki',
  [ViewMode.Skills]: 'knowledgeDetails.skills',
  [ViewMode.Tree]: 'knowledgeDetails.navTree',
  [ViewMode.Graph]: 'knowledgeDetails.structureGraph',
  [ViewMode.MindMap]: 'knowledgeDetails.structureMindmap',
  [ViewMode.Timeline]: 'knowledgeDetails.structureTimeline',
  // [ViewMode.SessionEssence]: 'knowledgeDetails.structureSessionEssence',
  // [ViewMode.SessionGraph]: 'knowledgeDetails.structureSessionGraph',
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
