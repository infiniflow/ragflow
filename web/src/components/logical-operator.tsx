import { useBuildSwitchLogicOperatorOptions } from '@/hooks/logic-hooks/use-build-options';
import { RAGFlowFormItem } from './ragflow-form';
import { RAGFlowSelect } from './ui/select';

type LogicalOperatorProps = { name: string };

export function LogicalOperator({ name }: LogicalOperatorProps) {
  const switchLogicOperatorOptions = useBuildSwitchLogicOperatorOptions();

  return (
    <div className="relative min-w-14">
      <RAGFlowFormItem
        name={name}
        className="absolute top-1/2 -translate-y-1/2 right-1 left-0 z-10 bg-bg-base"
      >
        <RAGFlowSelect
          options={switchLogicOperatorOptions}
          triggerClassName="w-full text-xs px-1 py-0 h-6"
        ></RAGFlowSelect>
      </RAGFlowFormItem>
      <div className="absolute border-l border-y w-5 right-0 top-4 bottom-4 rounded-l-lg"></div>
    </div>
  );
}
