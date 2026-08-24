/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
