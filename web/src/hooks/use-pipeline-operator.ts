import { useFetchPipelineDslByPipelineId } from '@/hooks/use-agent-request';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import {
  buildParserConfigFromNodes,
  buildPipelineOperatorNodes,
} from '@/utils/pipeline-operator';
import { useEffect, useMemo, useRef, useState } from 'react';
import { UseFormReturn } from 'react-hook-form';

export const usePipelineOperatorNodes = (
  pipelineId?: string,
  pipelineParserConfig?: Record<string, any>,
  isBuiltin = false,
) => {
  const { dsl, loading } = useFetchPipelineDslByPipelineId(
    pipelineId,
    isBuiltin,
  );

  const operatorNodes = useMemo(() => {
    return buildPipelineOperatorNodes(dsl, pipelineParserConfig);
  }, [dsl, pipelineParserConfig]);

  return { operatorNodes, loading };
};

/**
 * Resets parser_config when the selected pipeline changes, so stale configs
 * from the previous pipeline are never submitted. Once the new pipeline's
 * DSL has loaded, parser_config is seeded with the DSL defaults (i.e. what
 * the operator tabs display).
 *
 * The very first pipeline seen is treated as the initial load: the form was
 * already reset with the saved parser_config on mount, so it is left
 * untouched.
 */
export const useResetParserConfigOnPipelineChange = (
  form: UseFormReturn<any>,
  pipelineId: string | undefined,
  savedPipelineId: string | undefined,
  operatorNodes: RAGFlowNodeType[],
) => {
  const previousPipelineIdRef = useRef<string>();

  useEffect(() => {
    if (!pipelineId) {
      // Selection cleared (e.g. parse type switched): drop configs that
      // belonged to the previously selected pipeline.
      if (previousPipelineIdRef.current) {
        previousPipelineIdRef.current = '';
        form.setValue('parser_config', {});
      }
      return;
    }

    const isInitialLoad =
      previousPipelineIdRef.current === undefined &&
      pipelineId === savedPipelineId;
    if (isInitialLoad || previousPipelineIdRef.current === pipelineId) {
      previousPipelineIdRef.current = pipelineId;
      return;
    }

    if (operatorNodes.length === 0) {
      // The new pipeline's DSL is still loading — clear the previous
      // pipeline's configs right away; defaults are seeded once it arrives.
      form.setValue('parser_config', {});
      return;
    }

    previousPipelineIdRef.current = pipelineId;
    form.setValue('parser_config', buildParserConfigFromNodes(operatorNodes));
  }, [form, pipelineId, savedPipelineId, operatorNodes]);
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
