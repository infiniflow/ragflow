import { Collapse } from '@/components/collapse';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import { Textarea } from '@/components/ui/textarea';
import { buildOptions } from '@/utils/form';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  RateLimitPerList,
  WebhookContentType,
  WebhookExecutionMode,
  WebhookMaxBodySize,
  WebhookMethod,
  WebhookSecurityAuthType,
} from '../../../constant';
import { DynamicStringForm } from '../../components/dynamic-string-form';
import { Auth } from './auth';
import { WebhookRequestSchema } from './request-schema';
import { WebhookResponse } from './response';

const RateLimitPerOptions = buildOptions(RateLimitPerList);

export function WebHook() {
  const { t } = useTranslation();
  const form = useFormContext();

  const executionMode = useWatch({
    control: form.control,
    name: 'execution_mode',
  });

  return (
    <>
      <RAGFlowFormItem name="methods" label={t('flow.webhook.methods')}>
        {(field) => (
          <MultiSelect
            options={buildOptions(WebhookMethod)}
            placeholder={t('fileManager.pleaseSelect')}
            maxCount={100}
            onValueChange={field.onChange}
            defaultValue={field.value}
            modalPopover
          />
        )}
      </RAGFlowFormItem>
      <RAGFlowFormItem
        name="content_types"
        label={t('flow.webhook.contentTypes')}
      >
        <SelectWithSearch
          options={buildOptions(WebhookContentType)}
        ></SelectWithSearch>
      </RAGFlowFormItem>
      <Collapse title={<div>Security</div>}>
        <section className="space-y-4">
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
            <SelectWithSearch
              options={buildOptions(WebhookMaxBodySize)}
            ></SelectWithSearch>
          </RAGFlowFormItem>
          <DynamicStringForm
            name="security.ip_whitelist"
            label={t('flow.webhook.ipWhitelist')}
          ></DynamicStringForm>
        </section>
      </Collapse>
      <WebhookRequestSchema></WebhookRequestSchema>
      <RAGFlowFormItem
        name="schema"
        label={t('flow.webhook.schema')}
        className="hidden"
      >
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
      {executionMode === WebhookExecutionMode.Immediately && (
        <WebhookResponse></WebhookResponse>
      )}
    </>
  );
}
