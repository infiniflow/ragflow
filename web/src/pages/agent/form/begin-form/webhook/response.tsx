import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { useTranslation } from 'react-i18next';
import { DynamicResponse } from './dynamic-response';

export function WebhookResponse() {
  const { t } = useTranslation();

  return (
    <>
      <Separator></Separator>
      <section className="space-y-4">
        <RAGFlowFormItem
          name={'response.status'}
          label={t('flow.webhook.status')}
        >
          <Input type="number"></Input>
        </RAGFlowFormItem>
        <DynamicResponse
          name="response.headers_template"
          label={t('flow.webhook.headersTemplate')}
        ></DynamicResponse>
        <DynamicResponse
          name="response.body_template"
          label={t('flow.webhook.bodyTemplate')}
        ></DynamicResponse>
      </section>
    </>
  );
}
