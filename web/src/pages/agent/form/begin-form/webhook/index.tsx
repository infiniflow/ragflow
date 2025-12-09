import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { buildOptions } from '@/utils/form';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  WebhookContentType,
  WebhookExecutionMode,
  WebhookMethod,
  WebhookSecurityAuthType,
} from '../../../constant';
import { DynamicStringForm } from '../../components/dynamic-string-form';
import { SchemaDialog } from '../../components/schema-dialog';
import { SchemaPanel } from '../../components/schema-panel';
import { useShowSchemaDialog } from '../use-show-schema-dialog';
import { Auth } from './auth';
import { WebhookResponse } from './response';

const RateLimitPerOptions = buildOptions(['minute', 'hour', 'day']);

export function WebHook() {
  const { t } = useTranslation();
  const form = useFormContext();

  const executionMode = useWatch({
    control: form.control,
    name: 'execution_mode',
  });

  const {
    showSchemaDialog,
    schemaDialogVisible,
    hideSchemaDialog,
    handleSchemaDialogOk,
  } = useShowSchemaDialog(form);

  const schema = form.getValues('schema');

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
      <Separator></Separator>
      <>
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
      </>
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
      <Separator></Separator>
      <section className="flex justify-between items-center">
        Schema
        <Button variant={'ghost'} onClick={showSchemaDialog}>
          {t('flow.structuredOutput.configuration')}
        </Button>
      </section>
      <SchemaPanel value={schema}></SchemaPanel>
      {schemaDialogVisible && (
        <SchemaDialog
          initialValues={schema}
          hideModal={hideSchemaDialog}
          onOk={handleSchemaDialogOk}
        ></SchemaDialog>
      )}
    </>
  );
}
