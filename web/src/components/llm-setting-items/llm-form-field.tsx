import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { useTranslation } from 'react-i18next';
import { SelectWithSearch } from '../originui/select-with-search';
import { RAGFlowFormItem } from '../ragflow-form';

export type LLMFormFieldProps = {
  options?: any[];
  name?: string;
};

export function LLMFormField({ options, name }: LLMFormFieldProps) {
  const { t } = useTranslation();

  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  return (
    <RAGFlowFormItem name={name || 'llm_id'} label={t('chat.model')}>
      <SelectWithSearch options={options || modelOptions}></SelectWithSearch>
    </RAGFlowFormItem>
  );
}
