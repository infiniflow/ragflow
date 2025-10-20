import { IRagNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { needsSingleStepDebugging, showCopyIcon } from '../../utils';
import { CommonHandle, LeftEndHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
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
    <ToolBar
      selected={selected}
      id={id}
      label={data.label}
      showRun={needsSingleStepDebugging(data.label)}
      showCopy={showCopyIcon(data.label)}
    >
      <NodeWrapper selected={selected}>
        <LeftEndHandle></LeftEndHandle>
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
    </ToolBar>
  );
}

export const RagNode = memo(InnerRagNode);
