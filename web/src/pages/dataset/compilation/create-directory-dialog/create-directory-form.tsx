'use client';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useTranslation } from 'react-i18next';

export function CreateDirectoryFormFields() {
  const { t } = useTranslation();

  return (
    <div className="space-y-4">
      <RAGFlowFormItem
        name="name"
        label={t('knowledgeDetails.directoryName')}
        required
      >
        <Input placeholder={t('common.pleaseInput')} autoComplete="off" />
      </RAGFlowFormItem>
      <RAGFlowFormItem name="rule" label={t('knowledgeDetails.directoryRule')}>
        <Textarea placeholder={t('common.pleaseInput')} />
      </RAGFlowFormItem>
    </div>
  );
}
