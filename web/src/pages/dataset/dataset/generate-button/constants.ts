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
}

export enum TraceType {
  Graph = 'graph',
  Raptor = 'raptor',
  Artifact = 'artifact',
  Skill = 'skill',
}

export const GenerateTypeMap = {
  [GenerateType.KnowledgeGraph]: ProcessingType.knowledgeGraph,
  [GenerateType.Raptor]: ProcessingType.raptor,
  [GenerateType.Artifact]: ProcessingType.artifact,
  [GenerateType.ToSkills]: ProcessingType.skill,
};
