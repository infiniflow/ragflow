import { Collapse } from '@/components/collapse';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { useTranslation } from 'react-i18next';
import { DynamicResponse } from './dynamic-response';

export function WebhookResponse() {
  const { t } = useTranslation();

  return (
    <Collapse title={<div>Response</div>}>
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
    </Collapse>
  );
}
