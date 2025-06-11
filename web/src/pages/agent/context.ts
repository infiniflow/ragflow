import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { createContext } from 'react';
import { useAddNode } from './hooks/use-add-node';
import { useCacheChatLog } from './hooks/use-cache-chat-log';
import { useShowLogSheet } from './hooks/use-show-drawer';

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

type AgentChatContextType = Pick<
  ReturnType<typeof useShowLogSheet>,
  'showLogSheet'
>;

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
