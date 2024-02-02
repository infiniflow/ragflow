import { KnowledgeRouteKey } from '@/constants/knowledge';

export const routeMap = {
  [KnowledgeRouteKey.Dataset]: 'Dataset',
  [KnowledgeRouteKey.Testing]: 'Retrieval testing',
  [KnowledgeRouteKey.Configration]: 'Configuration',
};

export * from '@/constants/knowledge';
