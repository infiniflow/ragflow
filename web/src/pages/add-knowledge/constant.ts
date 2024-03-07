import { KnowledgeRouteKey } from '@/constants/knowledge';

export const routeMap = {
  [KnowledgeRouteKey.Dataset]: 'Dataset',
  [KnowledgeRouteKey.Testing]: 'Retrieval testing',
  [KnowledgeRouteKey.Configuration]: 'Configuration',
};

export enum KnowledgeDatasetRouteKey {
  Chunk = 'chunk',
  File = 'file',
}

export const datasetRouteMap = {
  [KnowledgeDatasetRouteKey.Chunk]: 'Chunk',
  [KnowledgeDatasetRouteKey.File]: 'File Upload',
};

export * from '@/constants/knowledge';
