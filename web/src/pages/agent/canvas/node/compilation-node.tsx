import { IRagNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { get } from 'lodash';
import { CompilationTemplateLabelCard, LLMLabelCard } from './card';
import { RagNode } from './index';

export function CompilationNode({ ...props }: NodeProps<IRagNode>) {
  const { data } = props;
  const groupId = get(data, 'form.compilation_template_group_id');
  const llmId = get(data, 'form.llm_id');

  return (
    <RagNode {...props}>
      <section className="flex flex-col gap-2">
        <LLMLabelCard llmId={llmId}></LLMLabelCard>
        <CompilationTemplateLabelCard
          groupId={groupId}
        ></CompilationTemplateLabelCard>
      </section>
    </RagNode>
  );
}
