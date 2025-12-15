import { Collapse } from '@/components/collapse';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { WebhookContentType } from '@/pages/agent/constant';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';
import { DynamicRequest } from './dynamic-request';

export function WebhookRequestSchema() {
  const { t } = useTranslation();

  return (
    <Collapse title={<div>{t('flow.webhook.schema')}</div>}>
      <section className="space-y-4">
        <RAGFlowFormItem
          name="content_types"
          label={t('flow.webhook.contentTypes')}
        >
          <SelectWithSearch
            options={buildOptions(WebhookContentType)}
          ></SelectWithSearch>
        </RAGFlowFormItem>
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
