import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';
import {
  WebhookContentType,
  WebhookExecutionMode,
  WebhookMethod,
  WebhookSecurityAuthType,
} from '../../../constant';
import { DynamicStringForm } from '../../components/dynamic-string-form';
import { Auth } from './auth';

const RateLimitPerOptions = buildOptions(['minute', 'hour', 'day']);

export function WebHook() {
  const { t } = useTranslation();

  return (
    <>
      <RAGFlowFormItem name="methods" label={t('flow.webhook.methods')}>
        <SelectWithSearch
          options={buildOptions(WebhookMethod)}
        ></SelectWithSearch>
      </RAGFlowFormItem>
      <RAGFlowFormItem
        name="content_types"
        label={t('flow.webhook.contentTypes')}
      >
        <SelectWithSearch
          options={buildOptions(WebhookContentType)}
        ></SelectWithSearch>
      </RAGFlowFormItem>
      <section className="space-y-5 bg-bg-card p-2 rounded">
        <RAGFlowFormItem
          name="security.auth_type"
          label={t('flow.webhook.authType')}
        >
          <SelectWithSearch
            options={buildOptions(WebhookSecurityAuthType)}
          ></SelectWithSearch>
        </RAGFlowFormItem>
        <Auth></Auth>
        <RAGFlowFormItem
          name="security.rate_limit.limit"
          label={t('flow.webhook.limit')}
        >
          <Input type="number"></Input>
        </RAGFlowFormItem>
        <RAGFlowFormItem
          name="security.rate_limit.per"
          label={t('flow.webhook.per')}
        >
          <SelectWithSearch options={RateLimitPerOptions}></SelectWithSearch>
        </RAGFlowFormItem>
        <RAGFlowFormItem
          name="security.max_body_size"
          label={t('flow.webhook.maxBodySize')}
        >
          <Input></Input>
        </RAGFlowFormItem>
        <DynamicStringForm
          name="security.ip_whitelist"
          label={t('flow.webhook.ipWhitelist')}
        ></DynamicStringForm>
      </section>
      <RAGFlowFormItem name="schema" label={t('flow.webhook.schema')}>
        <Textarea></Textarea>
      </RAGFlowFormItem>
      <RAGFlowFormItem name="response" label={t('flow.webhook.response')}>
        <Textarea></Textarea>
      </RAGFlowFormItem>
      <RAGFlowFormItem
        name="execution_mode"
        label={t('flow.webhook.executionMode')}
      >
        <SelectWithSearch
          options={buildOptions(WebhookExecutionMode)}
        ></SelectWithSearch>
      </RAGFlowFormItem>
    </>
  );
}
