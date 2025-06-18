import { IToolNode } from '@/interfaces/database/agent';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { CommonHandle } from './handle';
import { LeftHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerToolNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IToolNode>) {
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper>
        <CommonHandle
          id={NodeHandleId.End}
          type="target"
          position={Position.Top}
          isConnectable={isConnectable}
          style={LeftHandleStyle}
          nodeId={id}
        ></CommonHandle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
      </NodeWrapper>
    </ToolBar>
  );
}

export const ToolNode = memo(InnerToolNode);
