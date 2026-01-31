import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { PermissionRole } from '@/constants/permission';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export function PermissionFormField() {
  const { t } = useTranslation();
  const teamOptions = useMemo(() => {
    return Object.values(PermissionRole).map((x) => ({
      label: t('knowledgeConfiguration.' + x),
      value: x,
    }));
  }, [t]);

  return (
    <div className="items-center">
      <RAGFlowFormItem
        name="permission"
        label={t('knowledgeConfiguration.permissions')}
        tooltip={t('knowledgeConfiguration.permissionsTip')}
        horizontal={true}
      >
        <SelectWithSearch
          options={teamOptions}
          triggerClassName="w-full"
        ></SelectWithSearch>
      </RAGFlowFormItem>
    </div>
  );
}
