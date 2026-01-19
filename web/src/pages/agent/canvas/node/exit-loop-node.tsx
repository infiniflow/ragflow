import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { LeftEndHandle } from './handle';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

export function ExitLoopNode({ id, data, selected }: NodeProps<BaseNode<any>>) {
  return (
    <ToolBar
      selected={selected}
      id={id}
      label={data.label}
      showRun={false}
      showCopy={false}
    >
      <NodeWrapper selected={selected} id={id}>
        <LeftEndHandle></LeftEndHandle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
      </NodeWrapper>
    </ToolBar>
  );
}
