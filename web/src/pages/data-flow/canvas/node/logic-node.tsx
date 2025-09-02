import { ILogicNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

export function InnerLogicNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<ILogicNode>) {
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected}>
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
          style={RightHandleStyle}
          id="b"
          nodeId={id}
        ></CommonHandle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
      </NodeWrapper>
    </ToolBar>
  );
}

export const LogicNode = memo(InnerLogicNode);
