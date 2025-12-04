import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Textarea } from '@/components/ui/textarea';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';
import {
  WebhookContentType,
  WebhookExecutionMode,
  WebhookMethod,
} from '../../constant';

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
      <RAGFlowFormItem name="security" label={t('flow.webhook.security')}>
        <Textarea></Textarea>
      </RAGFlowFormItem>
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
