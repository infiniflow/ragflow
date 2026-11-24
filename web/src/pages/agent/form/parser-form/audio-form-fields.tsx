import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import { useTranslation } from 'react-i18next';
import { useResourceTenantId } from '../../context';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function AudioFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const resourceTenantId = useResourceTenantId();

  return (
    <>
      {/* Multimodal Model */}
      <ModelTreeSelectFormField
        name={buildFieldNameWithPrefix('vlm.llm_id', prefix)}
        label={t('chat.model')}
        modelTypes={ModelTypeMap.asr_id}
        allowClear
        resourceTenantId={resourceTenantId}
      />
    </>
  );
}
