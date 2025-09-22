import { crossLanguageOptions } from '@/components/cross-language-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { LLMFormField } from '@/components/llm-setting-items/llm-form-field';
import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';
import { FileType } from '../../constant';
import { OutputFormatMap } from './constant';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

function buildOutputOptionsFormatMap() {
  return Object.entries(OutputFormatMap).reduce<
    Record<string, SelectWithSearchFlagOptionType[]>
  >((pre, [key, value]) => {
    pre[key] = buildOptions(value);
    return pre;
  }, {});
}

export type OutputFormatFormFieldProps = CommonProps & {
  fileType: FileType;
};

export function OutputFormatFormField({
  prefix,
  fileType,
}: OutputFormatFormFieldProps) {
  const { t } = useTranslation();
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`output_format`, prefix)}
      label={t('dataflow.outputFormat')}
    >
      <SelectWithSearch
        options={buildOutputOptionsFormatMap()[fileType]}
      ></SelectWithSearch>
    </RAGFlowFormItem>
  );
}

export function ParserMethodFormField({
  prefix,
  optionsWithoutLLM,
}: CommonProps & { optionsWithoutLLM?: { value: string; label: string }[] }) {
  const { t } = useTranslation();
  return (
    <LayoutRecognizeFormField
      name={buildFieldNameWithPrefix(`parse_method`, prefix)}
      horizontal={false}
      optionsWithoutLLM={optionsWithoutLLM}
      label={t('dataflow.parserMethod')}
    ></LayoutRecognizeFormField>
  );
}

export function LargeModelFormField({ prefix }: CommonProps) {
  return (
    <LLMFormField
      name={buildFieldNameWithPrefix('llm_id', prefix)}
    ></LLMFormField>
  );
}

export function LanguageFormField({ prefix }: CommonProps) {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`lang`, prefix)}
      label={t('dataflow.lang')}
    >
      {(field) => (
        <SelectWithSearch
          options={crossLanguageOptions}
          value={field.value}
          onChange={field.onChange}
        ></SelectWithSearch>
      )}
    </RAGFlowFormItem>
  );
}
