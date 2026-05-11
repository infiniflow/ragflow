import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/use-llm-request';
import { useWatch } from 'react-hook-form';
import {
  FlattenMediaToTextFormField,
  LargeModelFormField,
  OutputFormatFormFieldProps,
  RmdirFormField,
} from './common-form-fields';
import { buildFieldNameWithPrefix } from './utils';

export function WordFormFields({ prefix }: OutputFormatFormFieldProps) {
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
