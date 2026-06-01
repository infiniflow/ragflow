import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  FlattenMediaToTextFormField,
  RemoveHeaderFooterFormField,
  RmdirFormField,
} from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function TextMarkdownFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const flattenMediaToText = useWatch({
    name: buildFieldNameWithPrefix('flatten_media_to_text', prefix),
  });

  return (
    <>
      <RmdirFormField prefix={prefix} />
      <FlattenMediaToTextFormField prefix={prefix} />
      {!flattenMediaToText && (
        <ModelTreeSelectFormField
          name={buildFieldNameWithPrefix('vlm.llm_id', prefix)}
          label={t('chat.model')}
          modelTypes={ModelTypeMap.img2txt_id}
          allowClear
        />
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
