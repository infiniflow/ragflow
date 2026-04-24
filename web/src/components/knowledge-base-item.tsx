import { DocumentParserType } from '@/constants/knowledge';
import { useFetchKnowledgeList } from '@/hooks/use-knowledge-request';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { useBuildQueryVariableOptions } from '@/pages/agent/hooks/use-get-begin-query';
import { toLower } from 'lodash';
import { useMemo } from 'react';
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
  const { list: datasetListOrigin } = useFetchKnowledgeList(true);
  const form = useFormContext();
  const datasetId = useWatch({ name, control: form.control });

  const selectedEmbedId = useMemo(() => {
    const data = datasetListOrigin?.find((item) => item.id === datasetId?.[0]);
    return data?.embedding_model ?? '';
  }, [datasetId, datasetListOrigin]);

  const nextOptions = useMemo(() => {
    const datasetListMap = datasetListOrigin
      .filter((x) => x.chunk_method !== DocumentParserType.Tag)
      .map((item: IKnowledge) => {
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
              <DatasetLabel text={item.embedding_model} />
            </section>
          ),
          value: item.id,
          disabled:
            item.embedding_model !== selectedEmbedId && selectedEmbedId !== '',
        };
      });

    return datasetListMap;
  }, [datasetListOrigin, selectedEmbedId]);

  return {
    datasetOptions: nextOptions,
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

  const { datasetOptions } = useDisableDifferenceEmbeddingDataset(name);

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
          return {
            ...x,
            options: x.options
              .filter((y) => toLower(y.type).includes('string'))
              .map((x) => ({
                ...x,
                icon: () => (
                  <RAGFlowAvatar
                    className="size-4 mr-2"
                    avatar={x.label}
                    name={x.label}
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
          {...field}
        />
      )}
    </RAGFlowFormItem>
  );
}
