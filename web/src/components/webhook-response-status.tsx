import { useTranslation } from 'react-i18next';
import { RAGFlowFormItem } from './ragflow-form';
import { Input } from './ui/input';

type WebHookResponseStatusFormFieldProps = {
  name: string;
};

export function WebHookResponseStatusFormField({
  name,
}: WebHookResponseStatusFormFieldProps) {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem name={name} label={t('flow.webhook.status')}>
      <Input type="number"></Input>
    </RAGFlowFormItem>
  );
}
