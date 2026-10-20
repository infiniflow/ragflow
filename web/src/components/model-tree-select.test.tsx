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

// The component module pulls in the model-list hook, which transitively imports
// the router (createBrowserRouter needs a global Request this environment does
// not provide). buildModelTree is pure, so stubbing the hook keeps the import
// graph from booting the app shell.
jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: () => ({
    data: [],
    isFetched: true,
    isError: false,
    loading: false,
  }),
}));

// eslint-disable-next-line import/first
import { buildModelTree } from './model-tree-select';

// Declared locally rather than imported as IAddedModel: this suite's transformer
// rejects a type that arrives through a value import. Only the fields the tree
// reads are modelled.
function addedModel(overrides: Record<string, unknown>) {
  return {
    model_id: '',
    model_type: ['chat'],
    name: 'm',
    provider_id: 'p',
    provider_name: 'Provider',
    instance_id: 'i',
    instance_name: 'default',
    ...overrides,
  };
}

function build(models: unknown[], types: string[], providers?: string[]) {
  return buildModelTree(models as never, types, undefined, providers);
}

/** Flattens the tree into leaf ids so assertions do not depend on tree shape. */
function leaves(nodes: ReturnType<typeof build>): string[] {
  const out: string[] = [];
  const walk = (list: ReturnType<typeof build>) => {
    for (const node of list) {
      if (node.children?.length) walk(node.children as typeof list);
      else out.push(node.id);
    }
  };
  walk(nodes);
  return out;
}

describe('buildModelTree', () => {
  it('selects by model type', () => {
    const models = [
      addedModel({ name: 'chat-model', model_type: ['chat'] }),
      addedModel({
        name: 'decision-model',
        model_type: ['decision'],
        provider_name: 'SystemOne',
      }),
    ];

    const onlyDecision = leaves(build(models, ['decision'], ['SystemOne']));
    expect(onlyDecision).toHaveLength(1);
    expect(onlyDecision[0]).toContain('decision-model');
  });

  // The decision slot speaks TypeSafe System One, which only SystemOne
  // implements. A decision-typed model from another provider would be accepted
  // by the picker and then fail on the first call.
  it('drops decision models from providers outside the allowlist', () => {
    const models = [
      addedModel({
        name: 'systemone-model',
        model_type: ['decision'],
        provider_name: 'SystemOne',
      }),
      addedModel({
        name: 'other-model',
        model_type: ['decision'],
        provider_name: 'OpenAI-API-Compatible',
      }),
    ];

    const tree = build(models, ['decision'], ['SystemOne']);
    expect(tree.map((node) => node.title)).toEqual(['SystemOne']);
    expect(leaves(tree)).toHaveLength(1);
  });

  it('leaves types without an allowlist unrestricted', () => {
    const models = [
      addedModel({
        name: 'a',
        model_type: ['chat'],
        provider_name: 'ProviderA',
      }),
      addedModel({
        name: 'b',
        model_type: ['chat'],
        provider_name: 'ProviderB',
      }),
    ];

    const tree = build(models, ['chat']);
    expect(tree.map((node) => node.title).sort()).toEqual([
      'ProviderA',
      'ProviderB',
    ]);
  });
});
