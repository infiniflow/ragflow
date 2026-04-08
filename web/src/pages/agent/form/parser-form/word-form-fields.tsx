import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/use-llm-request';
import {
  LargeModelFormField,
  OutputFormatFormFieldProps,
  RmdirFormField,
} from './common-form-fields';

export function WordFormFields({ prefix }: OutputFormatFormFieldProps) {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Image2text,
  ]);

  return (
    <>
      <RmdirFormField prefix={prefix} />
      {/* Multimodal Model */}
      <LargeModelFormField
        prefix={prefix}
        options={modelOptions}
      ></LargeModelFormField>
    </>
  );
}
