import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Switch } from '@/components/ui/switch';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FileType } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import {
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
        <RAGFlowFormItem
          name={buildFieldNameWithPrefix('extract_automatic_numbering', prefix)}
          label={t('flow.extractAutomaticNumbering')}
          tooltip={t('flow.extractAutomaticNumberingTip')}
          horizontal={true}
          labelClassName="w-full"
          valueClassName="w-8"
        >
          {(field) => (
            <Switch
              checked={field.value ?? false}
              onCheckedChange={field.onChange}
            />
          )}
        </RAGFlowFormItem>
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
