import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { ParserFormSchemaType } from '../../form/parser-form';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';

function ParserNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<BaseNode<ParserFormSchemaType>>) {
  return (
    <NodeWrapper selected={selected} id={id}>
      <CommonHandle
        id={NodeHandleId.End}
        type="target"
        position={Position.Left}
        isConnectable={isConnectable}
        style={LeftHandleStyle}
        nodeId={id}
      ></CommonHandle>
      <CommonHandle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        id={NodeHandleId.Start}
        style={RightHandleStyle}
        nodeId={id}
        isConnectableEnd={false}
      ></CommonHandle>
      <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
    </NodeWrapper>
  );
}

export default memo(ParserNode);
