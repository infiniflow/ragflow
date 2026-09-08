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
import { normalizeOperatorForm } from '@/utils/pipeline-operator';
import { memo, useCallback, useMemo } from 'react';
import { FieldErrors } from 'react-hook-form';
import PipelineOperatorForm from './pipeline-operator-form';

type PipelineOperatorTabsProps = {
  nodes: RAGFlowNodeType[];
  activeTab: string;
  onTabChange: (tab: string) => void;
  onOperatorValuesChange: (operatorId: string, values: any) => void;
  // Current values of the outer form's parser_config, keyed by operatorId.
  // The outer form is the single source of truth: Radix unmounts inactive
  // tabs, so a remounted operator form must initialize from these values —
  // not from the static node form built off the pipeline DSL — otherwise
  // unsaved edits are lost on tab switches.
  operatorValues?: Record<string, any>;
  // Validation errors from the outer form's parser_config, keyed by
  // operatorId; each entry is mirrored onto the operator form's fields.
  operatorFormErrors?: Record<string, FieldErrors | undefined>;
};

const PipelineOperatorTabs = ({
  nodes,
  activeTab,
  onTabChange,
  onOperatorValuesChange,
  operatorValues,
  operatorFormErrors,
}: PipelineOperatorTabsProps) => {
  const getOperatorId = useCallback((node: RAGFlowNodeType) => {
    return (
      (node.data as Record<string, any>)?.operatorId || node.data?.label || ''
    );
  }, []);

  const mergedNodes = useMemo(() => {
    if (!operatorValues) {
      return nodes;
    }
    return nodes.map((node) => {
      const operatorId = getOperatorId(node);
      const values = operatorValues[operatorId];
      if (!values || typeof values !== 'object') {
        return node;
      }
      return {
        ...node,
        data: {
          ...node.data,
          form: normalizeOperatorForm(operatorId, values),
        },
      };
    });
  }, [nodes, operatorValues, getOperatorId]);

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
    <Tabs value={activeTab} onValueChange={onTabChange} className="w-full">
      <TabsList className="w-full justify-start">
        {mergedNodes.map((node, index) => {
          const tabValue = getTabValue(node, index);
          return (
            <TabsTrigger key={tabValue} value={tabValue}>
              {node.data?.name || node.data?.label || tabValue}
            </TabsTrigger>
          );
        })}
      </TabsList>
      {mergedNodes.map((node, index) => {
        const tabValue = getTabValue(node, index);
        return (
          <TabsContent key={tabValue} value={tabValue}>
            <PipelineOperatorForm
              node={node}
              onValuesChange={handleValuesChange(node)}
              externalErrors={operatorFormErrors?.[getOperatorId(node)]}
            />
          </TabsContent>
        );
      })}
    </Tabs>
  );
};

export default memo(PipelineOperatorTabs);
