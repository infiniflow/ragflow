import { IRagNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { get } from 'lodash';
import { LLMLabelCard } from './card';
import { RagNode } from './index';

export function ExtractorNode({ ...props }: NodeProps<IRagNode>) {
  const { data } = props;

  return (
    <RagNode {...props}>
      <LLMLabelCard llmId={get(data, 'form.llm_id')}></LLMLabelCard>
    </RagNode>
  );
}
