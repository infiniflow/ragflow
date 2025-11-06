import { IRagNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { RagNode } from '.';

export function VariableAggregatorNode({ ...props }: NodeProps<IRagNode>) {
  return (
    <RagNode {...props}>
      <section>VariableAggregatorNode</section>
    </RagNode>
  );
}
