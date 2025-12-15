import { Collapse } from '@/components/collapse';
import CopyToClipboard from '@/components/copy-to-clipboard';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import { Textarea } from '@/components/ui/textarea';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
import {
  RateLimitPerList,
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
  const { id } = useParams();

  const text = `${location.protocol}//${location.host}/api/v1/webhook/${id}`;

  return (
    <>
      <div className="bg-bg-card p-1 rounded-md flex gap-2">
        <span className="flex-1 truncate">{text}</span>
        <CopyToClipboard text={text}></CopyToClipboard>
      </div>
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

      <WebhookResponse></WebhookResponse>
    </>
  );
}
