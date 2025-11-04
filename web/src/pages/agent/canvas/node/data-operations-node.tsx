import { IRagNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { RagNode } from '.';

export function DataOperationsNode({ ...props }: NodeProps<IRagNode>) {
  return (
    <RagNode {...props}>
      <section>select</section>
    </RagNode>
  );
}
