import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { memo } from 'react';
import { InnerIterationNode, InnerIterationStartNode } from './iteration-node';

export function InnerLoopNode({ ...props }: NodeProps<BaseNode<any>>) {
  return <InnerIterationNode {...props}></InnerIterationNode>;
}

export const LoopNode = memo(InnerLoopNode);

export function InnerLoopStartNode({ ...props }: NodeProps<BaseNode<any>>) {
  return <InnerIterationStartNode {...props}></InnerIterationStartNode>;
}

export const LoopStartNode = memo(InnerLoopStartNode);
