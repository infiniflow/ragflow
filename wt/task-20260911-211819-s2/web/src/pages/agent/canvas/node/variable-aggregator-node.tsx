import { NodeCollapsible } from '@/components/collapse';
import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps } from '@xyflow/react';
import { RagNode } from '.';
import { VariableAggregatorFormSchemaType } from '../../form/variable-aggregator-form/schema';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { LabelCard } from './card';

export function VariableAggregatorNode({
  ...props
}: NodeProps<BaseNode<VariableAggregatorFormSchemaType>>) {
  const { data } = props;
  const { getLabel } = useGetVariableLabelOrTypeByValue();

  return (
    <RagNode {...props}>
      <NodeCollapsible items={data.form?.groups}>
        {(x, idx) => (
          <section key={idx} className="space-y-1">
            <div className="flex justify-between items-center gap-2">
              <span className="flex-1 min-w-0 truncate"> {x.group_name}</span>
              <span className="text-text-secondary">{x.type}</span>
            </div>
            <div className="space-y-1">
              {x.variables?.map((y, index) => (
                <LabelCard key={index} className="truncate">
                  {getLabel(y.value)}
                </LabelCard>
              ))}
            </div>
          </section>
        )}
      </NodeCollapsible>
    </RagNode>
  );
}
