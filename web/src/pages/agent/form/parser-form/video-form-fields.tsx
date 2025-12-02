import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/use-llm-request';
import {
  LargeModelFormField,
  OutputFormatFormFieldProps,
} from './common-form-fields';

export function AudioFormFields({ prefix }: OutputFormatFormFieldProps) {
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

export function VideoFormFields({ prefix }: OutputFormatFormFieldProps) {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Image2text,
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
