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
}

export const GenerateTypeMap = {
  [GenerateType.KnowledgeGraph]: ProcessingType.knowledgeGraph,
  [GenerateType.Raptor]: ProcessingType.raptor,
};

export const IconKeyMap = {
  KnowledgeGraph: 'knowledgegraph',
  Raptor: 'dataflow-01',
};
