import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { memo, useCallback } from 'react';
import PipelineOperatorForm from './pipeline-operator-form';

type PipelineOperatorTabsProps = {
  nodes: RAGFlowNodeType[];
  value: string;
  onValueChange: (value: string) => void;
  onOperatorValuesChange: (operatorId: string, values: any) => void;
};

const PipelineOperatorTabs = ({
  nodes,
  value,
  onValueChange,
  onOperatorValuesChange,
}: PipelineOperatorTabsProps) => {
  const getOperatorId = useCallback((node: RAGFlowNodeType) => {
    return (
      (node.data as Record<string, any>)?.operatorId || node.data?.label || ''
    );
  }, []);

  const getTabValue = useCallback(
    (node: RAGFlowNodeType, index: number) => {
      return getOperatorId(node) || String(index);
    },
    [getOperatorId],
  );

  const handleValuesChange = useCallback(
    (node: RAGFlowNodeType) => (values: any) => {
      onOperatorValuesChange(getOperatorId(node), values);
    },
    [getOperatorId, onOperatorValuesChange],
  );

  return (
    <Tabs value={value} onValueChange={onValueChange} className="w-full">
      <TabsList className="w-full justify-start">
        {nodes.map((node, index) => {
          const tabValue = getTabValue(node, index);
          return (
            <TabsTrigger key={tabValue} value={tabValue}>
              {node.data?.name || node.data?.label || tabValue}
            </TabsTrigger>
          );
        })}
      </TabsList>
      {nodes.map((node, index) => {
        const tabValue = getTabValue(node, index);
        return (
          <TabsContent key={tabValue} value={tabValue}>
            <PipelineOperatorForm
              node={node}
              onValuesChange={handleValuesChange(node)}
            />
          </TabsContent>
        );
      })}
    </Tabs>
  );
};

export default memo(PipelineOperatorTabs);
