import type { DSL } from '@/interfaces/database/agent';
import { buildPipelineOperatorNodes } from '@/utils/pipeline-operator';
import { useMemo } from 'react';

type PipelineOperatorNodesPrivateParams = {
  dsl?: DSL;
  savedParserConfig?: Record<string, any>;
};

export function usePipelineOperatorNodesPrivate({
  dsl,
  savedParserConfig,
}: PipelineOperatorNodesPrivateParams) {
  const operatorNodes = useMemo(
    () => buildPipelineOperatorNodes(dsl, savedParserConfig),
    [dsl, savedParserConfig],
  );

  return { operatorNodes, loading: false };
}
