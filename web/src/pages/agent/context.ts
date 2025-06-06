import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { createContext } from 'react';

export const AgentFormContext = createContext<RAGFlowNodeType | undefined>(
  undefined,
);
