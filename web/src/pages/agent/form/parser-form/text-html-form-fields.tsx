import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/use-llm-request';
import { useWatch } from 'react-hook-form';
import {
  FlattenMediaToTextFormField,
  LargeModelFormField,
  RemoveHeaderFooterFormField,
  RmdirFormField,
} from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function TextMarkdownFormFields({ prefix }: CommonProps) {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Image2text,
  ]);
  const flattenMediaToText = useWatch({
    name: buildFieldNameWithPrefix('flatten_media_to_text', prefix),
  });

  return (
    <>
      <RmdirFormField prefix={prefix} />
      <FlattenMediaToTextFormField prefix={prefix} />
      {!flattenMediaToText && (
        <LargeModelFormField
          prefix={prefix}
          options={modelOptions}
        ></LargeModelFormField>
      )}
    </>
  );
}

export function HtmlFormFields({ prefix }: CommonProps) {
  return (
    <>
      <RmdirFormField prefix={prefix} />
      <RemoveHeaderFooterFormField prefix={prefix} />
    </>
  );
}
