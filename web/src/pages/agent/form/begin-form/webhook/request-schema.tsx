import { Collapse } from '@/components/collapse';
import { useTranslation } from 'react-i18next';
import { DynamicRequest } from './dynamic-request';

export function WebhookRequestSchema() {
  const { t } = useTranslation();

  return (
    <Collapse title={<div>{t('flow.webhook.schema')}</div>}>
      <section className="space-y-4">
        <DynamicRequest
          name="schema.query"
          label={t('flow.webhook.queryParameters')}
        ></DynamicRequest>
        <DynamicRequest
          name="schema.headers"
          label={t('flow.webhook.headerParameters')}
        ></DynamicRequest>
        <DynamicRequest
          name="schema.body"
          isObject
          label={t('flow.webhook.requestBodyParameters')}
        ></DynamicRequest>
      </section>
    </Collapse>
  );
}
