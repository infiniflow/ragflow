import type { RAGFlowNodeType } from '@/interfaces/database/agent';
import { PropsWithChildren } from 'react';

type ParserFormContentProps = PropsWithChildren<{
  node?: RAGFlowNodeType;
}>;

export function ParserFormContent({ children }: ParserFormContentProps) {
  return children;
}
