import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { createContext } from 'react';

export const FlowFormContext = createContext<RAGFlowNodeType | undefined>(
  undefined,
);
