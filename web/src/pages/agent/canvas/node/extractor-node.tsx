import LLMLabel from '@/components/llm-select/llm-label';
import { IRagNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { get } from 'lodash';
import { LabelCard } from './card';
import { RagNode } from './index';

export function ExtractorNode({ ...props }: NodeProps<IRagNode>) {
  const { data } = props;

  return (
    <RagNode {...props}>
      <LabelCard>
        <LLMLabel value={get(data, 'form.llm_id')}></LLMLabel>
      </LabelCard>
    </RagNode>
  );
}
