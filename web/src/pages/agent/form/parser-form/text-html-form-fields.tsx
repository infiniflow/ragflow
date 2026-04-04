import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/use-llm-request';
import { LargeModelFormField, RmdirFormField } from './common-form-fields';
import { CommonProps } from './interface';

export function TextMarkdownFormFields({ prefix }: CommonProps) {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Image2text,
  ]);

  return (
    <>
      <RmdirFormField prefix={prefix} />
      <LargeModelFormField
        prefix={prefix}
        options={modelOptions}
      ></LargeModelFormField>
    </>
  );
}

export function HtmlFormFields({ prefix }: CommonProps) {
  return <RmdirFormField prefix={prefix} />;
}
