import { NodeCollapsible } from '@/components/collapse';
import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { RagNode } from '.';
import { VariableAssignerFormSchemaType } from '../../form/variable-assigner-form';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { LabelCard } from './card';

export function VariableAssignerNode({
  ...props
}: NodeProps<BaseNode<VariableAssignerFormSchemaType>>) {
  const { data } = props;
  const { getLabel } = useGetVariableLabelOrTypeByValue();

  return (
    <RagNode {...props}>
      <NodeCollapsible items={data.form?.variables}>
        {(x, idx) => (
          <section key={idx} className="space-y-1">
            <LabelCard key={idx} className="flex justify-between gap-2">
              <span className="flex truncate min-w-0">
                {getLabel(x.variable)}
              </span>
              <span className="border px-1 rounded-sm">{x.operator}</span>
            </LabelCard>
          </section>
        )}
      </NodeCollapsible>
    </RagNode>
  );
}
