import { LLMFormField } from '@/components/llm-setting-items/llm-form-field';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function LanguageFormField({ prefix }: CommonProps) {
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`lang`, prefix)}
      label="lang"
    >
      <SelectWithSearch options={[]}></SelectWithSearch>
    </RAGFlowFormItem>
  );
}

export function OutputFormatFormField({ prefix }: CommonProps) {
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`output_format`, prefix)}
      label="output_format"
    >
      <SelectWithSearch options={[]}></SelectWithSearch>
    </RAGFlowFormItem>
  );
}

export function ParserMethodFormField({ prefix }: CommonProps) {
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
