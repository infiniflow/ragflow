import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import i18n from '@/locales/config';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

export const NameFormSchema = {
  name: z
    .string()
    .min(1, {
      message: i18n.t('common.namePlaceholder'),
    })
    .trim(),
};

export function NameFormField() {
  const { t } = useTranslation();
  return (
    <RAGFlowFormItem name="name" required label={t('common.name')}>
      <Input placeholder={t('common.namePlaceholder')} autoComplete="off" />
    </RAGFlowFormItem>
  );
}
