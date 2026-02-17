import { DocumentParserType } from '@/constants/knowledge';
import { useFetchKnowledgeList } from '@/hooks/use-knowledge-request';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { useBuildQueryVariableOptions } from '@/pages/agent/hooks/use-get-begin-query';
import { toLower } from 'lodash';
import { useEffect, useMemo, useState } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { RAGFlowAvatar } from './ragflow-avatar';
import { FormControl, FormField, FormItem, FormLabel } from './ui/form';
import { MultiSelect, MultiSelectOptionType } from './ui/multi-select';

function buildQueryVariableOptionsByShowVariable(showVariable?: boolean) {
  return showVariable ? useBuildQueryVariableOptions : () => [];
}

export function useDisableDifferenceEmbeddingDataset() {
  const [datasetOptions, setDatasetOptions] = useState<MultiSelectOptionType[]>(
    [],
  );
  const [datasetSelectEmbedId, setDatasetSelectEmbedId] = useState('');
  const { list: datasetListOrigin } = useFetchKnowledgeList(true);

  useEffect(() => {
    const datasetListMap = datasetListOrigin
      .filter((x) => x.parser_id !== DocumentParserType.Tag)
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
            <div className="text-xs px-4 p-1 bg-bg-card text-text-secondary rounded-lg border border-bg-card">
              {item.embd_id}
            </div>
          ),
          value: item.id,
          disabled:
            item.embd_id !== datasetSelectEmbedId &&
            datasetSelectEmbedId !== '',
        };
      });
    setDatasetOptions(datasetListMap);
  }, [datasetListOrigin, datasetSelectEmbedId]);

  const handleDatasetSelectChange = (
    value: string[],
    onChange: (value: string[]) => void,
  ) => {
    if (value.length) {
      const data = datasetListOrigin?.find((item) => item.id === value[0]);
      setDatasetSelectEmbedId(data?.embd_id ?? '');
    } else {
      setDatasetSelectEmbedId('');
    }
    onChange?.(value);
  };

  return {
    datasetOptions,
    handleDatasetSelectChange,
  };
}

export function KnowledgeBaseFormField({
  showVariable = false,
}: {
  showVariable?: boolean;
}) {
  const form = useFormContext();
  const { t } = useTranslation();

  const { datasetOptions, handleDatasetSelectChange } =
    useDisableDifferenceEmbeddingDataset();

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
    <FormField
      control={form.control}
      name="kb_ids"
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('chat.knowledgeBasesTip')}>
            {t('chat.knowledgeBases')}
          </FormLabel>
          <FormControl>
            <MultiSelect
              options={options}
              onValueChange={(value) => {
                handleDatasetSelectChange(value, field.onChange);
              }}
              placeholder={t('chat.knowledgeBasesMessage')}
              variant="inverted"
              maxCount={100}
              defaultValue={field.value}
              showSelectAll={false}
              {...field}
            />
          </FormControl>
        </FormItem>
      )}
    />
  );
}
