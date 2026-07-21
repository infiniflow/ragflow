import { useCompilationTemplateGroupOptions } from '@/hooks/use-compilation-template-group-request';
import { IRagNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { get } from 'lodash';
import { LabelCard, LLMLabelCard } from './card';
import { RagNode } from './index';
import { useTranslation } from 'react-i18next';

export function CompilationNode({ ...props }: NodeProps<IRagNode>) {
  const { data } = props;
  const { t } = useTranslation();
  const options = useCompilationTemplateGroupOptions();
  const groupId = get(data, 'form.compilation_template_group_ids');
  const groupName =
    options.find((option) => option.value === groupId)?.label ?? groupId;

  return (
    <RagNode {...props}>
      <section className="flex flex-col gap-2">
        <LLMLabelCard llmId={get(data, 'form.llm_id')}></LLMLabelCard>
        <LabelCard className="text-text-primary flex justify-between flex-col gap-1">
          <span className="text-text-secondary">
            {t('knowledgeConfiguration.compilationTemplate')}
          </span>
          <div className="truncate">{groupName}</div>
        </LabelCard>
      </section>
    </RagNode>
  );
}
