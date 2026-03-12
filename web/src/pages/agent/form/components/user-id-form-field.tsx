import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useTranslation } from 'react-i18next';
import { PromptEditor } from './prompt-editor';

export function UserIdFormField() {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem name="user_id" label={t('flow.userId')}>
      <PromptEditor multiLine={false} showToolbar={false}></PromptEditor>
    </RAGFlowFormItem>
  );
}
