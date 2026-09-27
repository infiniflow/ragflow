import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import { useTranslation } from 'react-i18next';
import { useOwnerTenantId } from '../../context';
import { OutputFormatFormFieldProps } from './common-form-fields';
import { buildFieldNameWithPrefix } from './utils';

export function AudioFormFields({ prefix }: OutputFormatFormFieldProps) {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();

  return (
    <>
      {/* Multimodal Model */}
      <ModelTreeSelectFormField
        name={buildFieldNameWithPrefix('vlm.llm_id', prefix)}
        label={t('chat.model')}
        modelTypes={ModelTypeMap.asr_id}
        allowClear
        ownerTenantId={ownerTenantId}
      />
    </>
  );
}

export function VideoFormFields({ prefix }: OutputFormatFormFieldProps) {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();

  return (
    <>
      {/* Multimodal Model */}
      <ModelTreeSelectFormField
        name={buildFieldNameWithPrefix('vlm.llm_id', prefix)}
        label={t('chat.model')}
        modelTypes={ModelTypeMap.img2txt_id}
        allowClear
        ownerTenantId={ownerTenantId}
      />
    </>
  );
}
