import { Collapse } from '@/components/collapse';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import {
  WebhookContentType,
  WebhookRequestParameters,
} from '@/pages/agent/constant';
import { buildOptions } from '@/utils/form';
import { useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { DynamicRequest } from './dynamic-request';

export function WebhookRequestSchema() {
  const { t } = useTranslation();
  const form = useFormContext();
  const contentType = useWatch({
    name: 'content_types',
    control: form.control,
  });
  const isFormDataContentType =
    contentType === WebhookContentType.MultipartFormData;

  const bodyOperatorList = useMemo(() => {
    return isFormDataContentType
      ? [
          WebhookRequestParameters.String,
          WebhookRequestParameters.Number,
          WebhookRequestParameters.Boolean,
          WebhookRequestParameters.File,
        ]
      : [
          WebhookRequestParameters.String,
          WebhookRequestParameters.Number,
          WebhookRequestParameters.Boolean,
        ];
  }, [isFormDataContentType]);

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
          operatorList={[
            WebhookRequestParameters.String,
            WebhookRequestParameters.Number,
            WebhookRequestParameters.Boolean,
          ]}
        ></DynamicRequest>
        <DynamicRequest
          name="schema.headers"
          label={t('flow.webhook.headerParameters')}
          operatorList={[WebhookRequestParameters.String]}
        ></DynamicRequest>
        <DynamicRequest
          name="schema.body"
          operatorList={bodyOperatorList}
          label={t('flow.webhook.requestBodyParameters')}
        ></DynamicRequest>
      </section>
    </Collapse>
  );
}
