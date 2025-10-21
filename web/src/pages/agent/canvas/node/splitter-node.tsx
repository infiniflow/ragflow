import { IRagNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { PropsWithChildren, memo } from 'react';
import { NodeHandleId, Operator } from '../../constant';
import OperatorIcon from '../../operator-icon';
import { LabelCard } from './card';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

type RagNodeProps = NodeProps<IRagNode> & PropsWithChildren;
function InnerSplitterNode({
  id,
  data,
  isConnectable = true,
  selected,
}: RagNodeProps) {
  return (
    <ToolBar
      selected={selected}
      id={id}
      label={data.label}
      showCopy={false}
      showRun={false}
    >
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
        <NodeHeader
          id={id}
          name={'Chunker'}
          label={data.label}
          icon={<OperatorIcon name={Operator.Splitter}></OperatorIcon>}
        ></NodeHeader>
        <LabelCard>{data.name}</LabelCard>
      </NodeWrapper>
    </ToolBar>
  );
}

export const SplitterNode = memo(InnerSplitterNode);
