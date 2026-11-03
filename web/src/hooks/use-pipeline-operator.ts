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

import { useFetchPipelineDslByPipelineId } from '@/hooks/use-agent-request';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import {
  buildParserConfigFromNodes,
  buildPipelineOperatorNodes,
} from '@/utils/pipeline-operator';
import { useEffect, useMemo, useRef, useState } from 'react';
import { UseFormReturn } from 'react-hook-form';

export const usePipelineOperatorNodes = ({
  pipelineId,
  versionId,
  savedParserConfig,
  isBuiltin = false,
}: {
  pipelineId?: string;
  // Selected canvas version id; ignored for builtin pipelines. Without it no
  // version is fetched and operatorNodes stays empty (tabs hidden).
  versionId?: string;
  // Saved parser_config, passed only while the selection still matches the
  // saved pipeline+version binding; otherwise defaults come from the DSL.
  savedParserConfig?: Record<string, any>;
  isBuiltin?: boolean;
}) => {
  const { dsl, loading } = useFetchPipelineDslByPipelineId(
    pipelineId,
    versionId,
    isBuiltin,
  );

  const operatorNodes = useMemo(() => {
    return buildPipelineOperatorNodes(dsl, savedParserConfig);
  }, [dsl, savedParserConfig]);

  return { operatorNodes, loading };
};

/**
 * Resets parser_config when the selected pipeline binding (pipeline +
 * version) changes, so stale configs from the previous binding are never
 * submitted. Once the new version's DSL has loaded, parser_config is seeded
 * with the DSL defaults (i.e. what the operator tabs display).
 *
 * The very first binding seen is treated as the initial load: the form was
 * already reset with the saved parser_config on mount, so it is left
 * untouched when pipeline and version both match the saved binding.
 *
 * A `'' → versionId` transition within the same pipeline is the latest-
 * version prefill adopting a concrete version for an unpinned binding, not a
 * user switch — parser_config (which still holds the saved values) must not
 * be reset.
 */
export const useResetParserConfigOnPipelineChange = ({
  form,
  pipelineId,
  versionId,
  savedPipelineId,
  savedVersionId,
  operatorNodes,
}: {
  form: UseFormReturn<any>;
  pipelineId: string | undefined;
  versionId: string | undefined;
  savedPipelineId: string | undefined;
  savedVersionId: string | undefined;
  operatorNodes: RAGFlowNodeType[];
}) => {
  const previousBindingRef = useRef<string>();

  useEffect(() => {
    if (!pipelineId) {
      // Selection cleared (e.g. parse type switched): drop configs that
      // belonged to the previously selected pipeline.
      if (previousBindingRef.current) {
        previousBindingRef.current = '';
        form.setValue('parser_config', {});
      }
      return;
    }

    const binding = `${pipelineId}::${versionId ?? ''}`;
    const previousBinding = previousBindingRef.current;
    if (previousBinding === binding) {
      return;
    }

    if (previousBinding === undefined) {
      // Initial load: the form was already reset with the saved
      // parser_config on mount, so leave it untouched when the selection
      // still matches the saved binding.
      const matchesSavedBinding =
        pipelineId === savedPipelineId &&
        (versionId ?? '') === (savedVersionId ?? '');
      if (matchesSavedBinding) {
        previousBindingRef.current = binding;
        return;
      }
    } else {
      // Prefill adoption: a `'' → versionId` transition within the same
      // pipeline is the latest-version prefill, never a user switch.
      const [prevPipelineId, prevVersionId] = previousBinding.split('::');
      const isPrefillAdoption =
        prevPipelineId === pipelineId && prevVersionId === '' && !!versionId;
      if (isPrefillAdoption) {
        previousBindingRef.current = binding;
        return;
      }
    }

    if (operatorNodes.length === 0) {
      // The new binding's DSL is still loading — clear the previous
      // configs right away but keep the previous binding in the ref, so
      // the arrival of operatorNodes re-fires this effect and seeds the
      // defaults then.
      form.setValue('parser_config', {});
      return;
    }

    previousBindingRef.current = binding;
    form.setValue('parser_config', buildParserConfigFromNodes(operatorNodes));
  }, [
    form,
    pipelineId,
    versionId,
    savedPipelineId,
    savedVersionId,
    operatorNodes,
  ]);
};

export const useActiveTab = (operatorNodes: RAGFlowNodeType[]) => {
  const [activeTab, setActiveTab] = useState('');

  useEffect(() => {
    if (operatorNodes.length > 0) {
      const firstTab =
        (operatorNodes[0].data as Record<string, any>)?.operatorId ||
        operatorNodes[0].data?.label ||
        '';
      const validTabs = operatorNodes.map(
        (node) =>
          (node.data as Record<string, any>)?.operatorId ||
          node.data?.label ||
          '',
      );
      setActiveTab((prev) => (validTabs.includes(prev) ? prev : firstTab));
    } else {
      setActiveTab('');
    }
  }, [operatorNodes]);

  return { activeTab, setActiveTab };
};
