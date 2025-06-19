import { IToolNode } from '@/interfaces/database/agent';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { NodeWrapper } from './node-wrapper';

function InnerToolNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IToolNode>) {
  return (
    <NodeWrapper>
      <Handle
        id={NodeHandleId.End}
        type="target"
        position={Position.Top}
        isConnectable={isConnectable}
      ></Handle>
    </NodeWrapper>
  );
}

export const ToolNode = memo(InnerToolNode);
