import { Collapse } from '@/components/collapse';
import { CopyToClipboardWithText } from '@/components/copy-to-clipboard';
import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Label } from '@/components/ui/label';
import { Modal } from '@/components/ui/modal/modal';
import { MultiSelect } from '@/components/ui/multi-select';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import message from '@/components/ui/message';
import {
  useCreateAgentWebhookCredential,
  useRevokeAgentWebhookCredential,
} from '@/hooks/use-agent-request';
import { buildOptions } from '@/utils/form';
import { upperFirst } from 'lodash';
import { useCallback, useState } from 'react';
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
  const [personalWebhookUrl, setPersonalWebhookUrl] = useState('');
  const { createWebhookCredential, loading: creatingCredential } =
    useCreateAgentWebhookCredential();
  const { revokeWebhookCredential, loading: revokingCredential } =
    useRevokeAgentWebhookCredential();

  const rateLimitPer = useWatch({
    name: 'security.rate_limit.per',
    control: form.control,
  });

  const getLimitRateLimitPerMax = useCallback((rateLimitPer: string) => {
    return RequestLimitMap[rateLimitPer as keyof typeof RequestLimitMap] ?? 100;
  }, []);

  const handleCreateCredential = useCallback(async () => {
    try {
      const credential = await createWebhookCredential();
      setPersonalWebhookUrl(
        new URL(credential.webhook_url, window.location.origin).toString(),
      );
      message.success(t('flow.webhook.credentialCreated'));
    } catch {
      message.error(t('flow.webhook.credentialRequestFailed'));
    }
  }, [createWebhookCredential, t]);

  const revokeCredential = useCallback(async () => {
    try {
      await revokeWebhookCredential();
      setPersonalWebhookUrl('');
      message.success(t('flow.webhook.credentialRevoked'));
    } catch {
      message.error(t('flow.webhook.credentialRequestFailed'));
    }
  }, [revokeWebhookCredential, t]);

  const handleRevokeCredential = useCallback(() => {
    Modal.confirm({
      title: t('flow.webhook.revokeCredentialTitle'),
      content: t('flow.webhook.revokeCredentialDescription'),
      okText: t('common.confirm'),
      cancelText: t('common.cancel'),
      okButtonClassName: 'bg-state-error hover:bg-state-error/90',
      onOk: revokeCredential,
    });
  }, [revokeCredential, t]);

  return (
    <>
      <section className="space-y-3 rounded-md border border-border-button bg-bg-card p-3">
        <div>
          <Label>{t('flow.webhook.personalUrl')}</Label>
          <p className="mt-1 text-xs text-text-secondary">
            {t('flow.webhook.credentialDescription')}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            loading={creatingCredential}
            onClick={handleCreateCredential}
          >
            {t('flow.webhook.createCredential')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            loading={revokingCredential}
            onClick={handleRevokeCredential}
          >
            {t('flow.webhook.revokeCredential')}
          </Button>
        </div>
        {personalWebhookUrl && (
          <CopyToClipboardWithText text={personalWebhookUrl} />
        )}
      </section>
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
                  integer
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
