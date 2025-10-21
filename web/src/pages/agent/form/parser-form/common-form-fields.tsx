import { crossLanguageOptions } from '@/components/cross-language-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import {
  LLMFormField,
  LLMFormFieldProps,
} from '@/components/llm-setting-items/llm-form-field';
import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { upperCase, upperFirst } from 'lodash';
import { useTranslation } from 'react-i18next';
import {
  FileType,
  OutputFormatMap,
  SpreadsheetOutputFormat,
} from '../../constant/pipeline';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

const UppercaseFields = [
  SpreadsheetOutputFormat.Html,
  SpreadsheetOutputFormat.Json,
];

function buildOutputOptionsFormatMap() {
  return Object.entries(OutputFormatMap).reduce<
    Record<string, SelectWithSearchFlagOptionType[]>
  >((pre, [key, value]) => {
    pre[key] = Object.values(value).map((v) => ({
      label: UppercaseFields.some((x) => x === v)
        ? upperCase(v)
        : upperFirst(v),
      value: v,
    }));
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
      label={t('flow.outputFormat')}
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
      label={t('flow.parserMethod')}
    ></LayoutRecognizeFormField>
  );
}

export function LargeModelFormField({
  prefix,
  options,
}: CommonProps & Pick<LLMFormFieldProps, 'options'>) {
  return (
    <LLMFormField
      name={buildFieldNameWithPrefix('llm_id', prefix)}
      options={options}
    ></LLMFormField>
  );
}

export function LanguageFormField({ prefix }: CommonProps) {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`lang`, prefix)}
      label={t('flow.lang')}
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
