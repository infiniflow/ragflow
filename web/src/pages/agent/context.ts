import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { createContext } from 'react';
import { useAddNode } from './hooks/use-add-node';

export const AgentFormContext = createContext<RAGFlowNodeType | undefined>(
  undefined,
);

type AgentInstanceContextType = Pick<
  ReturnType<typeof useAddNode>,
  'addCanvasNode'
>;

export const AgentInstanceContext = createContext<AgentInstanceContextType>(
  {} as AgentInstanceContextType,
);
