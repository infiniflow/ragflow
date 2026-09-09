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

import { DocumentParserType } from '@/constants/knowledge';
import {
  useFetchDatasetsByIds,
  useFetchKnowledgeList,
} from '@/hooks/use-knowledge-request';
import { renderHook } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { useDisableDifferenceEmbeddingDataset } from '../knowledge-base-item';

type IDataset = import('@/interfaces/database/dataset').IDataset;

jest.mock('@/hooks/use-knowledge-request', () => ({
  useFetchKnowledgeList: jest.fn(),
  useFetchDatasetsByIds: jest.fn(),
}));

jest.mock('@/pages/agent/hooks/use-get-begin-query', () => ({
  useBuildQueryVariableOptions: jest.fn(),
}));

function dataset(
  id: string,
  embedding_model: string,
  embedding_model_name?: string,
): IDataset {
  return {
    id,
    name: id,
    nickname: 'owner',
    chunk_count: 1,
    chunk_method: DocumentParserType.Naive,
    embedding_model,
    embedding_model_name,
  } as IDataset;
}

function renderDisabledOptions(
  list: IDataset[],
  selectedId?: string,
  selectedDatasets: IDataset[] = [],
) {
  jest.mocked(useFetchKnowledgeList).mockReturnValue({
    list,
    loading: false,
    fetchNextPage: jest.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
    handleScroll: jest.fn(),
  });
  jest.mocked(useFetchDatasetsByIds).mockReturnValue({
    data: selectedDatasets,
    loading: false,
  });

  function Wrapper({ children }: React.PropsWithChildren) {
    const form = useForm({
      defaultValues: { dataset_ids: selectedId ? [selectedId] : [] },
    });
    return <FormProvider {...form}>{children}</FormProvider>;
  }

  const { result } = renderHook(
    () => useDisableDifferenceEmbeddingDataset('dataset_ids'),
    { wrapper: Wrapper },
  );
  return Object.fromEntries(
    result.current.datasetOptions.map(({ value, disabled }) => [
      value,
      disabled,
    ]),
  );
}

describe('dataset embedding selection', () => {
  const ModelId = 'a'.repeat(32);
  const OtherModelId = 'b'.repeat(32);
  const Composite = 'bge-m3:latest@Ollama local@Ollama';

  it.each(['existing', 'new'])(
    'allows a composite and resolved model ID when %s is selected first',
    (selectedId) => {
      expect(
        renderDisabledOptions(
          [
            dataset('existing', Composite, ''),
            dataset('new', ModelId, Composite),
          ],
          selectedId,
        ),
      ).toEqual({ existing: false, new: false });
    },
  );

  it('compares resolved base names across provider instances', () => {
    expect(
      renderDisabledOptions(
        [
          dataset('selected', ModelId, Composite),
          dataset('same', OtherModelId, 'bge-m3:latest@remote@Ollama'),
          dataset('different', 'c'.repeat(32), 'nomic-embed-text@local@Ollama'),
        ],
        'selected',
      ),
    ).toEqual({ selected: false, same: false, different: true });
  });

  it.each([undefined, ''])(
    'falls back to the raw composite when the resolved name is %p',
    (resolvedName) => {
      expect(
        renderDisabledOptions(
          [
            dataset('selected', Composite, resolvedName),
            dataset('same', 'bge-m3:latest@remote@Ollama', resolvedName),
            dataset('different', 'nomic-embed-text@local@Ollama', resolvedName),
          ],
          'selected',
        ),
      ).toEqual({ selected: false, same: false, different: true });
    },
  );

  it('only groups unresolved model IDs by exact identity', () => {
    expect(
      renderDisabledOptions(
        [
          dataset('selected', ModelId, ''),
          dataset('same', ModelId),
          dataset('different', OtherModelId, ''),
          dataset('resolved', 'c'.repeat(32), Composite),
        ],
        'selected',
      ),
    ).toEqual({
      selected: false,
      same: false,
      different: true,
      resolved: true,
    });
  });

  it('preserves @ suffixes inside resolved model names', () => {
    expect(
      renderDisabledOptions(
        [
          dataset('selected', ModelId, 'nomic-embed-text@q8_0@local@LM-Studio'),
          dataset('same', 'nomic-embed-text@q8_0@remote@LM-Studio'),
          dataset(
            'different',
            OtherModelId,
            'nomic-embed-text@q4_0@local@LM-Studio',
          ),
        ],
        'selected',
      ),
    ).toEqual({ selected: false, same: false, different: true });
  });

  it('uses resolved names for selected datasets outside the current page', () => {
    expect(
      renderDisabledOptions(
        [
          dataset('same', Composite),
          dataset('different', 'other@local@Ollama'),
        ],
        'selected',
        [dataset('selected', ModelId, Composite)],
      ),
    ).toEqual({ same: false, different: true, selected: false });
  });

  it.each([undefined, 'selected'])(
    'keeps empty and tag datasets disabled with selection %p',
    (selectedId) => {
      expect(
        renderDisabledOptions(
          [
            dataset('selected', ModelId, Composite),
            { ...dataset('empty', ModelId, Composite), chunk_count: 0 },
            {
              ...dataset('tag', ModelId, Composite),
              chunk_method: DocumentParserType.Tag,
            },
            dataset('different', OtherModelId, 'other@local@Ollama'),
          ],
          selectedId,
        ),
      ).toEqual({
        selected: false,
        empty: true,
        tag: true,
        different: selectedId !== undefined,
      });
    },
  );
});
