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
  // Dataset-side embeddings show a fixed set of parser file types; only the
  // canvas parser allows add/remove.
  fixedFileFormats?: boolean;
};

const BuiltinOperatorLabels: Record<string, string> = {
  Parser_picture: 'Parser',
  Indexer_picture: 'Indexer',
  Parser_audio: 'Parser',
  Indexer_audio: 'Indexer',
  Parser_email: 'Parser',
  Indexer_email: 'Indexer',
};

function getOperatorTabLabel(node: RAGFlowNodeType, fallback: string) {
  const name = node.data?.name || '';
  const chunkerTemplate = name.match(/Chunker_(picture|audio|email)$/);
  if (chunkerTemplate) {
    const template = chunkerTemplate[1];
    return `${template[0].toUpperCase()}${template.slice(1)} Chunker`;
  }
  return BuiltinOperatorLabels[name] || name || node.data?.label || fallback;
}

const PipelineOperatorTabs = ({
  nodes,
  activeTab,
  onTabChange,
  onOperatorValuesChange,
  operatorValues,
  operatorFormErrors,
  fixedFileFormats,
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
              {getOperatorTabLabel(node, tabValue)}
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
              fixedFileFormats={fixedFileFormats}
            />
          </TabsContent>
        );
      })}
    </Tabs>
  );
};

export default memo(PipelineOperatorTabs);
