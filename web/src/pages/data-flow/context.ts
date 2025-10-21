import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { HandleType, Position } from '@xyflow/react';
import { createContext } from 'react';
import { useAddNode } from './hooks/use-add-node';
import { useShowFormDrawer } from './hooks/use-show-drawer';

export const AgentFormContext = createContext<RAGFlowNodeType | undefined>(
  undefined,
);

type AgentInstanceContextType = Pick<
  ReturnType<typeof useAddNode>,
  'addCanvasNode'
> &
  Pick<ReturnType<typeof useShowFormDrawer>, 'showFormDrawer'>;

export const AgentInstanceContext = createContext<AgentInstanceContextType>(
  {} as AgentInstanceContextType,
);

export type HandleContextType = {
  nodeId?: string;
  id?: string;
  type: HandleType;
  position: Position;
  isFromConnectionDrag: boolean;
};

export const HandleContext = createContext<HandleContextType>(
  {} as HandleContextType,
);

export type LogContextType = {
  messageId: string;
  setMessageId: (messageId: string) => void;
  setUploadedFileData: (data: Record<string, any>) => void;
};

export const LogContext = createContext<LogContextType>({} as LogContextType);
