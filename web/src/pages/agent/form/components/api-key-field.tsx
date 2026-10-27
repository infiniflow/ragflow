import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { t } from 'i18next';

interface IApiKeyFieldProps {
  placeholder?: string;
  required?: boolean;
}
export function ApiKeyField({ placeholder, required }: IApiKeyFieldProps) {
  return (
    <RAGFlowFormItem
      name="api_key"
      label={t('flow.apiKey')}
      required={required}
    >
      <Input type="password" placeholder={placeholder}></Input>
    </RAGFlowFormItem>
  );
}
