import { IRagNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { memo, useMemo } from 'react';
import { NodeHandleId, SingleOperators } from '../../constant';
import useGraphStore from '../../store';
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
  const getOperatorTypeFromId = useGraphStore(
    (state) => state.getOperatorTypeFromId,
  );

  const showCopy = useMemo(() => {
    const operatorName = getOperatorTypeFromId(id);
    return SingleOperators.every((x) => x !== operatorName);
  }, [getOperatorTypeFromId, id]);

  return (
    <ToolBar selected={selected} id={id} label={data.label} showCopy={showCopy}>
      <NodeWrapper selected={selected}>
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
    </ToolBar>
  );
}

export const RagNode = memo(InnerRagNode);
