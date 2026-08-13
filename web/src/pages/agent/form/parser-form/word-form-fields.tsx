import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FileType } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import {
  ExtractAutomaticNumberingFormField,
  FlattenMediaToTextFormField,
  OutputFormatFormFieldProps,
  RemoveHeaderFooterFormField,
  RmdirFormField,
} from './common-form-fields';
import { buildFieldNameWithPrefix } from './utils';

export function WordFormFields({
  prefix,
  fileType,
}: OutputFormatFormFieldProps) {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();
  const flattenMediaToText = useWatch({
    name: buildFieldNameWithPrefix('flatten_media_to_text', prefix),
  });

  return (
    <>
      <RmdirFormField prefix={prefix} />
      <RemoveHeaderFooterFormField prefix={prefix} />
      {fileType === FileType.Docx && (
        <ExtractAutomaticNumberingFormField prefix={prefix} />
      )}
      <FlattenMediaToTextFormField prefix={prefix} />
      {!flattenMediaToText && (
        <ModelTreeSelectFormField
          name={buildFieldNameWithPrefix('vlm.llm_id', prefix)}
          label={t('chat.model')}
          modelTypes={ModelTypeMap.img2txt_id}
          allowClear
          ownerTenantId={ownerTenantId}
        />
      )}
    </>
  );
}
