import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { LLMFormField } from '@/components/llm-setting-items/llm-form-field';
import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { buildOptions } from '@/utils/form';
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
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`output_format`, prefix)}
      label="output_format"
    >
      <SelectWithSearch
        options={buildOutputOptionsFormatMap()[fileType]}
      ></SelectWithSearch>
    </RAGFlowFormItem>
  );
}

export function ParserMethodFormField({ prefix }: CommonProps) {
  return (
    <LayoutRecognizeFormField
      name={buildFieldNameWithPrefix(`parse_method`, prefix)}
      horizontal={false}
    ></LayoutRecognizeFormField>
  );

  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`parse_method`, prefix)}
      label="parse_method"
    >
      <SelectWithSearch options={[]}></SelectWithSearch>
    </RAGFlowFormItem>
  );
}

export function LargeModelFormField({ prefix }: CommonProps) {
  return (
    <LLMFormField
      name={buildFieldNameWithPrefix('llm_id', prefix)}
    ></LLMFormField>
  );
}
