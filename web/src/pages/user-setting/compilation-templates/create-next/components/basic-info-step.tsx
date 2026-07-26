import { AvatarUpload } from '@/components/avatar-upload';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useTranslation } from 'react-i18next';

export function BasicInfoStep() {
  const { t } = useTranslation();

  return (
    <div className="max-w-2xl space-y-6 p-5 mx-auto w-full">
      <RAGFlowFormItem
        name="name"
        label={t('setting.groupName')}
        required
        horizontal
      >
        <Input placeholder={t('common.namePlaceholder')} />
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name="description"
        label={t('setting.groupDescription')}
        horizontal
      >
        <Textarea
          placeholder={t('common.descriptionPlaceholder')}
          rows={3}
          resize="vertical"
        />
      </RAGFlowFormItem>

      <RAGFlowFormItem name="avatar" label={t('setting.avatar')} horizontal>
        <AvatarUpload tips={t('knowledgeConfiguration.photoTip')} />
      </RAGFlowFormItem>
    </div>
  );
}
