import { Collapse } from '@/components/collapse';
import { CopyToClipboardWithText } from '@/components/copy-to-clipboard';
import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Label } from '@/components/ui/label';
import { MultiSelect } from '@/components/ui/multi-select';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { useBuildWebhookUrl } from '@/pages/agent/hooks/use-build-webhook-url';
import { buildOptions } from '@/utils/form';
import { upperFirst } from 'lodash';
import { useCallback } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  RateLimitPerList,
  WebhookMaxBodySize,
  WebhookMethod,
  WebhookRateLimitPer,
  WebhookSecurityAuthType,
} from '../../../constant';
import { DynamicStringForm } from '../../components/dynamic-string-form';
import { Auth } from './auth';
import { WebhookRequestSchema } from './request-schema';
import { WebhookResponse } from './response';

const RateLimitPerOptions = RateLimitPerList.map((x) => ({
  value: x,
  label: upperFirst(x),
}));

const RequestLimitMap = {
  [WebhookRateLimitPer.Second]: 100,
  [WebhookRateLimitPer.Minute]: 1000,
  [WebhookRateLimitPer.Hour]: 10000,
  [WebhookRateLimitPer.Day]: 100000,
};

export function WebHook() {
  const { t } = useTranslation();
  const form = useFormContext();

  const rateLimitPer = useWatch({
    name: 'security.rate_limit.per',
    control: form.control,
  });

  const getLimitRateLimitPerMax = useCallback((rateLimitPer: string) => {
    return RequestLimitMap[rateLimitPer as keyof typeof RequestLimitMap] ?? 100;
  }, []);

  const text = useBuildWebhookUrl();

  return (
    <>
      <CopyToClipboardWithText text={text}></CopyToClipboardWithText>
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
          <section>
            <Label>{t('flow.webhook.limit')}</Label>
            <div className="flex items-center mt-1 gap-2">
              <RAGFlowFormItem
                name="security.rate_limit.limit"
                className="flex-1"
              >
                <NumberInput
                  max={getLimitRateLimitPerMax(rateLimitPer)}
                  className="w-full"
                ></NumberInput>
              </RAGFlowFormItem>
              <Separator className="w-2" />
              <RAGFlowFormItem name="security.rate_limit.per">
                {(field) => (
                  <SelectWithSearch
                    options={RateLimitPerOptions}
                    value={field.value}
                    onChange={(val) => {
                      field.onChange(val);
                      form.setValue(
                        'security.rate_limit.limit',
                        getLimitRateLimitPerMax(val),
                      );
                    }}
                  ></SelectWithSearch>
                )}
              </RAGFlowFormItem>
            </div>
          </section>
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
