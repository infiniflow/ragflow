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
import { IDataset } from '@/interfaces/database/dataset';
import { useBuildQueryVariableOptions } from '@/pages/agent/hooks/use-get-begin-query';
import { getEmbeddingBaseName } from '@/utils/llm-util';
import { useDebounce } from 'ahooks';
import { toLower } from 'lodash';
import { type ReactNode, useCallback, useMemo, useState } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { RAGFlowAvatar } from './ragflow-avatar';
import { RAGFlowFormItem } from './ragflow-form';
import { MultiSelect } from './ui/multi-select';

function buildQueryVariableOptionsByShowVariable(showVariable?: boolean) {
  return showVariable ? useBuildQueryVariableOptions : () => [];
}

function DatasetLabel({ text }: { text: string }) {
  return (
    <div className="text-xs px-3 p-1 bg-bg-card text-text-secondary rounded-lg border border-bg-card">
      {text}
    </div>
  );
}

export function useDisableDifferenceEmbeddingDataset(name: string) {
  const form = useFormContext();
  const datasetId = useWatch({ name, control: form.control });
  const [searchString, setSearchString] = useState('');
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const {
    list: datasetListOrigin,
    loading,
    handleScroll,
    hasNextPage,
  } = useFetchKnowledgeList(false, debouncedSearchString);
  const selectedDatasetIds = useMemo(
    () => (Array.isArray(datasetId) ? datasetId : []),
    [datasetId],
  );

  // The paginated list may not contain the selected datasets (unloaded
  // pages, filtered out by the search box), so resolve them by ID to echo
  // their names back in the form field. A dataset that has been deleted
  // comes back missing and its badge falls back to the raw id.
  const { data: selectedDatasets } = useFetchDatasetsByIds(selectedDatasetIds);

  const datasetList = useMemo(() => {
    return Array.from(
      new Map(
        [...datasetListOrigin, ...(selectedDatasets ?? [])].map((dataset) => [
          dataset.id,
          dataset,
        ]),
      ).values(),
    );
  }, [datasetListOrigin, selectedDatasets]);

  // Datasets are mutually selectable when their embedding models resolve to
  // the same base model name, even if they use different provider instances
  // (e.g. "BAAI/bge-m3@renew@SILICONFLOW" vs "BAAI/bge-m3@COPY@SILICONFLOW").
  const selectedEmbedBaseName = useMemo(() => {
    const data = datasetList?.find((item) => item.id === datasetId?.[0]);
    return getEmbeddingBaseName(
      data?.embedding_model_name || data?.embedding_model,
    );
  }, [datasetId, datasetList]);

  const nextOptions = useMemo(() => {
    return datasetList.map((item: IDataset) => {
      return {
        label: item.name,
        icon: () => (
          <RAGFlowAvatar
            className="size-4"
            avatar={item.avatar}
            name={item.name}
          />
        ),
        suffix: (
          <section className="flex gap-2">
            <DatasetLabel text={item.nickname} />
            <DatasetLabel
              text={
                item.embedding_model_name
                  ? item.embedding_model_name
                  : item.embedding_model
              }
            />
          </section>
        ),
        value: item.id,
        disabled:
          item.chunk_count === 0 ||
          item.chunk_method === DocumentParserType.Tag ||
          (selectedEmbedBaseName !== '' &&
            getEmbeddingBaseName(
              item.embedding_model_name || item.embedding_model,
            ) !== selectedEmbedBaseName),
      };
    });
  }, [datasetList, selectedEmbedBaseName]);

  const handleSearchChange = useCallback((value: string) => {
    setSearchString(value);
  }, []);

  return {
    datasetOptions: nextOptions,
    handleSearchChange,
    loading,
    searchString,
    handleScroll,
    hasNextPage,
  };
}

export function KnowledgeBaseFormField({
  showVariable = false,
  name = 'dataset_ids',
  required = false,
}: {
  showVariable?: boolean;
  name?: string;
  required?: boolean;
}) {
  const { t } = useTranslation();

  const {
    datasetOptions,
    handleSearchChange,
    loading,
    searchString,
    handleScroll,
    hasNextPage,
  } = useDisableDifferenceEmbeddingDataset(name);

  const nextOptions = buildQueryVariableOptionsByShowVariable(showVariable)();

  const knowledgeOptions = datasetOptions;
  const options = useMemo(() => {
    if (showVariable) {
      return [
        {
          label: t('knowledgeDetails.dataset'),
          options: knowledgeOptions,
        },
        ...nextOptions.map((x) => {
          const groupLabel = (('label' in x
            ? x.label
            : 'title' in x
              ? x.title
              : '') ?? '') as ReactNode;

          return {
            ...x,
            label: groupLabel,
            options: x.options
              .filter((y) => toLower(y.type).includes('string'))
              .map((x) => ({
                ...x,
                label: x.label ?? x.value ?? '',
                value: x.value ?? '',
                icon: () => (
                  <RAGFlowAvatar
                    className="size-4 mr-2"
                    avatar={String(x.label ?? '')}
                    name={String(x.label ?? '')}
                  />
                ),
              })),
          };
        }),
      ];
    }

    return knowledgeOptions;
  }, [knowledgeOptions, nextOptions, showVariable, t]);

  return (
    <RAGFlowFormItem
      name={name}
      tooltip={t('chat.knowledgeBasesTip')}
      required={required}
      label={t('chat.knowledgeBases')}
    >
      {(field) => (
        <MultiSelect
          data-testid="chat-datasets-combobox"
          options={options}
          onValueChange={field.onChange}
          placeholder={t('chat.knowledgeBasesPlaceholder')}
          variant="inverted"
          maxCount={100}
          defaultValue={field.value}
          showSelectAll={false}
          popoverTestId="datasets-options"
          optionTestIdPrefix="datasets"
          searchValue={searchString}
          onSearchChange={handleSearchChange}
          isSearching={loading}
          shouldFilter={false}
          onListScroll={hasNextPage ? handleScroll : undefined}
          {...field}
        />
      )}
    </RAGFlowFormItem>
  );
}
