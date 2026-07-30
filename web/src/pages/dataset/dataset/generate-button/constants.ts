import { ProcessingType } from '../../dataset-overview/dataset-common';

export const GenerateStatus = {
  running: 'running',
  completed: 'completed',
  start: 'start',
  failed: 'failed',
};

export enum GenerateType {
  KnowledgeGraph = 'KnowledgeGraph',
  Raptor = 'Raptor',
  Artifact = 'Artifact',
  ToSkills = 'ToSkills',
  MindMap = 'MindMap',
  Timeline = 'Timeline',
  SessionEssence = 'SessionEssence',
  SessionGraph = 'SessionGraph',
}

export enum TraceType {
  Graph = 'graph',
  Raptor = 'raptor',
  Artifact = 'artifact',
  Skill = 'skill',
  MindMap = 'mindmap',
  Timeline = 'timeline',
  SessionEssence = 'session_essence',
  SessionGraph = 'session_graph',
}

export const GenerateTypeMap = {
  [GenerateType.KnowledgeGraph]: ProcessingType.knowledgeGraph,
  [GenerateType.Raptor]: ProcessingType.raptor,
  [GenerateType.Artifact]: ProcessingType.artifact,
  [GenerateType.ToSkills]: ProcessingType.skill,
  [GenerateType.MindMap]: ProcessingType.mindmap,
  [GenerateType.Timeline]: ProcessingType.timeline,
  [GenerateType.SessionEssence]: ProcessingType.sessionEssence,
  [GenerateType.SessionGraph]: ProcessingType.sessionGraph,
};
