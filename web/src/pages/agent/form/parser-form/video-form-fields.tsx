import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import {
  LargeModelFormField,
  OutputFormatFormFieldProps,
} from './common-form-fields';

export function VideoFormFields({ prefix }: OutputFormatFormFieldProps) {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Speech2text,
  ]);

  return (
    <>
      {/* Multimodal Model */}
      <LargeModelFormField
        prefix={prefix}
        options={modelOptions}
      ></LargeModelFormField>
    </>
  );
}
