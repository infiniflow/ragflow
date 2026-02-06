import { INodeEvent } from '@/hooks/use-send-message';
import { IMessage } from '@/interfaces/database/chat';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { HandleType, Position } from '@xyflow/react';
import { Dispatch, SetStateAction, createContext } from 'react';
import { useAddNode } from './hooks/use-add-node';
import { useCacheChatLog } from './hooks/use-cache-chat-log';
import { useShowFormDrawer, useShowLogSheet } from './hooks/use-show-drawer';

export const AgentFormContext = createContext<RAGFlowNodeType | undefined>(
  undefined,
);

type AgentInstanceContextType = Pick<
  ReturnType<typeof useAddNode>,
  'addCanvasNode'
> &
  Pick<ReturnType<typeof useShowFormDrawer>, 'showFormDrawer'> & {
    lastNode: INodeEvent | null;
    currentSendLoading: boolean;
    startButNotFinishedNodeIds: string[];
  };

export const AgentInstanceContext = createContext<AgentInstanceContextType>(
  {} as AgentInstanceContextType,
);

type AgentChatContextType = Pick<
  ReturnType<typeof useShowLogSheet>,
  'showLogSheet'
> & {
  setLastSendLoadingFunc: (loading: boolean, messageId: string) => void;
  setDerivedMessages: Dispatch<SetStateAction<IMessage[] | undefined>>;
};

export const AgentChatContext = createContext<AgentChatContextType>(
  {} as AgentChatContextType,
);

type AgentChatLogContextType = Pick<
  ReturnType<typeof useCacheChatLog>,
  'addEventList' | 'setCurrentMessageId'
>;

export const AgentChatLogContext = createContext<AgentChatLogContextType>(
  {} as AgentChatLogContextType,
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
