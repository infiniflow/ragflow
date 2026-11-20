import { AvatarNameDescription } from '@/components/avatar-name-description';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import PageRankFormField from '@/components/page-rank-form-field';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { LanguageTranslationMap } from '@/constants/common';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { showPermissionField } from '@/ee/constants/feature-flags';
import { useKnowledgeBaseContext } from '../contexts/knowledge-base-context';
import { EmbeddingModelItem } from './embedding-model-form-field';
import { PermissionFormField } from './permission-form-field';

export function GeneralForm() {
  const { t } = useTranslation();
  const { knowledgeBase } = useKnowledgeBaseContext();
  const resourceTenantId = knowledgeBase?.tenant_id;

  const languageOptions = useMemo(() => {
    return Object.keys(LanguageTranslationMap).map((x) => ({
      label: x,
      value: x,
    }));
  }, []);

  return (
    <>
      <AvatarNameDescription
        avatarField="avatar"
        descriptionPlaceholder={t('knowledgeConfiguration.datasetDescription')}
      />
      <div className="items-center">
        <RAGFlowFormItem
          name="language"
          label={t('common.language')}
          horizontal={true}
        >
          <SelectWithSearch
            options={languageOptions}
            triggerClassName="w-full"
            testId="ds-settings-basic-language-select"
          ></SelectWithSearch>
        </RAGFlowFormItem>
      </div>
      {showPermissionField && <PermissionFormField></PermissionFormField>}
      <EmbeddingModelItem
        isEdit={true}
        resourceTenantId={resourceTenantId}
        accessUserId={knowledgeBase?.user_id}
      ></EmbeddingModelItem>
      <PageRankFormField></PageRankFormField>
    </>
  );
}
