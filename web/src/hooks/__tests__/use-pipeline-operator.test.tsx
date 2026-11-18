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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { useFetchPipelineDslByPipelineId } from '../use-agent-request';
import { useResetParserConfigOnPipelineChange } from '../use-pipeline-operator';

// This file goes through the esbuild-jest babel pass, which cannot rewrite an
// imported binding that also appears in a type position — so node fixtures
// use this local structural type instead of importing RAGFlowNodeType.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type OperatorNode = any;

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: {
    fetchVersion: jest.fn(),
    getBuiltinPipeline: jest.fn(),
  },
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentService = require('@/services/agent-service').default;

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    {children}
  </QueryClientProvider>
);

const versionDsl = { graph: { nodes: [], edges: [] } };

beforeEach(() => {
  jest.clearAllMocks();
});

describe('useFetchPipelineDslByPipelineId', () => {
  it('fetches the builtin pipeline DSL directly, ignoring the version id', async () => {
    agentService.getBuiltinPipeline.mockResolvedValue({
      data: { data: { dsl: versionDsl } },
    });

    const { result } = renderHook(
      () => useFetchPipelineDslByPipelineId('general', undefined, true),
      { wrapper },
    );

    await waitFor(() => expect(result.current.dsl).toEqual(versionDsl));
    expect(agentService.getBuiltinPipeline).toHaveBeenCalledWith('general');
    expect(agentService.fetchVersion).not.toHaveBeenCalled();
  });

  it('fetches the selected canvas version for a dataflow pipeline', async () => {
    agentService.fetchVersion.mockResolvedValue({
      data: { code: 0, data: { id: 'version-1', dsl: versionDsl } },
    });

    const { result } = renderHook(
      () => useFetchPipelineDslByPipelineId('pipeline', 'version-1'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.dsl).toEqual(versionDsl));
    expect(agentService.fetchVersion).toHaveBeenCalledWith(
      {
        agentId: 'pipeline',
        versionId: 'version-1',
        skipGlobalErrorNotification: true,
      },
      true,
    );
    expect(agentService.getBuiltinPipeline).not.toHaveBeenCalled();
  });

  it('fetches nothing while the pipeline has no concrete version id', () => {
    const { result } = renderHook(
      () => useFetchPipelineDslByPipelineId('pipeline', ''),
      { wrapper },
    );

    expect(result.current.dsl).toBeUndefined();
    expect(result.current.loading).toBe(false);
    expect(agentService.fetchVersion).not.toHaveBeenCalled();
  });

  it('does not retry when the pinned version is gone', async () => {
    agentService.fetchVersion.mockResolvedValue({
      data: { code: 404, message: 'version not found' },
    });

    const { result } = renderHook(
      () => useFetchPipelineDslByPipelineId('pipeline', 'version-gone'),
      { wrapper },
    );

    // Wait long enough for a retry to have fired if one were configured.
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.dsl).toBeUndefined();
    expect(agentService.fetchVersion).toHaveBeenCalledTimes(1);
  });
});

// Drives a real RHF form through the reset hook with scripted operatorNodes,
// so the "DSL still loading" (empty nodes) and "DSL arrived" paths can be
// replayed without any network.
function renderResetForm(initialProps: {
  pipelineId?: string;
  versionId?: string;
  savedPipelineId?: string;
  savedVersionId?: string;
  operatorNodes: OperatorNode[];
  initialParserConfig?: Record<string, any>;
}) {
  return renderHook(
    (props) => {
      const form = useForm<{ parser_config: Record<string, any> }>({
        defaultValues: { parser_config: props.initialParserConfig ?? {} },
      });
      useResetParserConfigOnPipelineChange({
        form,
        pipelineId: props.pipelineId,
        versionId: props.versionId,
        savedPipelineId: props.savedPipelineId,
        savedVersionId: props.savedVersionId,
        operatorNodes: props.operatorNodes,
      });
      return form;
    },
    { initialProps },
  );
}

const operatorNode = (
  operatorId: string,
  form: Record<string, any>,
): OperatorNode => ({ id: operatorId, data: { operatorId, form } });

const savedConfig = { 'Parser:parse': { parse_method: 'deepdoc' } };

describe('useResetParserConfigOnPipelineChange', () => {
  it('leaves parser_config untouched on the initial load of the saved binding', () => {
    const { result } = renderResetForm({
      pipelineId: 'pipeline',
      versionId: 'version-1',
      savedPipelineId: 'pipeline',
      savedVersionId: 'version-1',
      operatorNodes: [operatorNode('Parser:parse', { parse_method: 'vision' })],
      initialParserConfig: savedConfig,
    });

    expect(result.current.getValues('parser_config')).toEqual(savedConfig);
  });

  it('keeps the saved parser_config across the latest-version prefill', () => {
    const nodes = [operatorNode('Parser:parse', { parse_method: 'vision' })];
    const { result, rerender } = renderResetForm({
      pipelineId: 'pipeline',
      versionId: '',
      savedPipelineId: 'pipeline',
      savedVersionId: '',
      operatorNodes: [],
      initialParserConfig: savedConfig,
    });
    expect(result.current.getValues('parser_config')).toEqual(savedConfig);

    // Prefill writes the latest concrete version id; the saved config must
    // survive because it predates the version concept.
    rerender({
      pipelineId: 'pipeline',
      versionId: 'version-latest',
      savedPipelineId: 'pipeline',
      savedVersionId: '',
      operatorNodes: [],
      initialParserConfig: savedConfig,
    });
    expect(result.current.getValues('parser_config')).toEqual(savedConfig);

    // The adopted version's DSL arriving changes nothing either.
    rerender({
      pipelineId: 'pipeline',
      versionId: 'version-latest',
      savedPipelineId: 'pipeline',
      savedVersionId: '',
      operatorNodes: nodes,
      initialParserConfig: savedConfig,
    });
    expect(result.current.getValues('parser_config')).toEqual(savedConfig);
  });

  it('reseeds parser_config from the new version after a version switch', () => {
    const nodesV1 = [operatorNode('Parser:parse', { parse_method: 'deepdoc' })];
    const nodesV2 = [operatorNode('Parser:parse', { parse_method: 'mineru' })];
    const base = {
      pipelineId: 'pipeline',
      savedPipelineId: 'pipeline',
      savedVersionId: 'version-1',
      initialParserConfig: savedConfig,
    };
    const { result, rerender } = renderResetForm({
      ...base,
      versionId: 'version-1',
      operatorNodes: nodesV1,
    });
    expect(result.current.getValues('parser_config')).toEqual(savedConfig);

    // Switch to v2: the new DSL is still loading, so the stale config is
    // cleared first.
    rerender({ ...base, versionId: 'version-2', operatorNodes: [] });
    expect(result.current.getValues('parser_config')).toEqual({});

    // Once v2's DSL arrives, its defaults are seeded.
    rerender({ ...base, versionId: 'version-2', operatorNodes: nodesV2 });
    expect(result.current.getValues('parser_config')).toEqual({
      'Parser:parse': { parse_method: 'mineru' },
    });
  });

  it('reseeds parser_config when switching to another pipeline', () => {
    const { result, rerender } = renderResetForm({
      pipelineId: 'pipeline-a',
      versionId: 'version-1',
      savedPipelineId: 'pipeline-a',
      savedVersionId: 'version-1',
      operatorNodes: [operatorNode('Parser:parse', {})],
      initialParserConfig: savedConfig,
    });

    rerender({
      pipelineId: 'pipeline-b',
      versionId: 'version-9',
      savedPipelineId: 'pipeline-a',
      savedVersionId: 'version-1',
      operatorNodes: [operatorNode('Tokenizer:tokenize', { model: 'gpt-4' })],
      initialParserConfig: savedConfig,
    });
    expect(result.current.getValues('parser_config')).toEqual({
      'Tokenizer:tokenize': { model: 'gpt-4' },
    });
  });

  it('clears parser_config when the selection is cleared', () => {
    const { result, rerender } = renderResetForm({
      pipelineId: 'pipeline',
      versionId: 'version-1',
      savedPipelineId: 'pipeline',
      savedVersionId: 'version-1',
      operatorNodes: [operatorNode('Parser:parse', {})],
      initialParserConfig: savedConfig,
    });

    rerender({
      pipelineId: undefined,
      versionId: undefined,
      savedPipelineId: 'pipeline',
      savedVersionId: 'version-1',
      operatorNodes: [],
      initialParserConfig: savedConfig,
    });
    expect(result.current.getValues('parser_config')).toEqual({});
  });

  it('does not mistake a re-selection after clearing for a prefill', () => {
    const { result, rerender } = renderResetForm({
      pipelineId: 'pipeline',
      versionId: 'version-1',
      savedPipelineId: 'pipeline',
      savedVersionId: 'version-1',
      operatorNodes: [operatorNode('Parser:parse', {})],
      initialParserConfig: savedConfig,
    });

    rerender({
      pipelineId: undefined,
      versionId: undefined,
      savedPipelineId: 'pipeline',
      savedVersionId: 'version-1',
      operatorNodes: [],
      initialParserConfig: savedConfig,
    });

    // Re-selecting the same pipeline with the same version is a fresh user
    // choice, so the (cleared) config is reseeded from the DSL defaults.
    rerender({
      pipelineId: 'pipeline',
      versionId: 'version-1',
      savedPipelineId: 'pipeline',
      savedVersionId: 'version-1',
      operatorNodes: [operatorNode('Parser:parse', { parse_method: 'mineru' })],
      initialParserConfig: savedConfig,
    });
    expect(result.current.getValues('parser_config')).toEqual({
      'Parser:parse': { parse_method: 'mineru' },
    });
  });
});
