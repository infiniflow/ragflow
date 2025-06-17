import { IRagNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerRagNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IRagNode>) {
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper>
        <CommonHandle
          id="c"
          type="source"
          position={Position.Left}
          isConnectable={isConnectable}
          style={LeftHandleStyle}
          nodeId={id}
        ></CommonHandle>
        <CommonHandle
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          id="b"
          style={RightHandleStyle}
          nodeId={id}
        ></CommonHandle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
      </NodeWrapper>
    </ToolBar>
  );
}

export const RagNode = memo(InnerRagNode);
