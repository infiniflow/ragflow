import {
  IIterationNode,
  IIterationStartNode,
} from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { NodeProps, NodeResizeControl, Position } from '@xyflow/react';
import { memo } from 'react';
import { NodeHandleId, Operator } from '../../constant';
import OperatorIcon from '../../operator-icon';
import { CommonHandle, LeftEndHandle } from './handle';
import styles from './index.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ResizeIcon, controlStyle } from './resize-icon';
import { ToolBar } from './toolbar';

export function InnerIterationNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IIterationNode>) {
  return (
    <ToolBar selected={selected} id={id} label={data.label} showRun={false}>
      <section
        className={cn('h-full bg-transparent rounded-b-md group', {
          [styles.selectedHeader]: selected,
        })}
      >
        <NodeResizeControl style={controlStyle} minWidth={100} minHeight={50}>
          <ResizeIcon />
        </NodeResizeControl>
        <LeftEndHandle></LeftEndHandle>
        <CommonHandle
          id={NodeHandleId.Start}
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          nodeId={id}
        ></CommonHandle>
        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          wrapperClassName={cn(
            'bg-background-header-bar p-2 rounded-t-[10px] absolute w-full top-[-44px] left-[-0.3px]',
            {
              [styles.selectedHeader]: selected,
            },
          )}
        ></NodeHeader>
      </section>
    </ToolBar>
  );
}

function InnerIterationStartNode({
  isConnectable = true,
  id,
  selected,
}: NodeProps<IIterationStartNode>) {
  return (
    <NodeWrapper className="w-20" selected={selected}>
      <CommonHandle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        isConnectableEnd={false}
        id={NodeHandleId.Start}
        nodeId={id}
      ></CommonHandle>
      <div>
        <OperatorIcon name={Operator.Begin}></OperatorIcon>
      </div>
    </NodeWrapper>
  );
}

export const IterationStartNode = memo(InnerIterationStartNode);

export const IterationNode = memo(InnerIterationNode);
