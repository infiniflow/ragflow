import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { LLMFactory } from '@/constants/llm';
import { buildOptions } from '@/utils/form';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

const algorithmOptions = buildOptions(['PaddleOCR-VL']);

export function PaddleOCROptionsFormField({
  namePrefix = 'parser_config',
}: {
  namePrefix?: string;
}) {
  const form = useFormContext();
  const { t } = useTranslation();
  const buildName = (field: string) =>
    namePrefix ? `${namePrefix}.${field}` : field;

  const layoutRecognize = useWatch({
    control: form.control,
    name: 'parser_config.layout_recognize',
  });

  // Check if PaddleOCR is selected (the value contains 'PaddleOCR' or matches the factory name)
  const isPaddleOCRSelected =
    layoutRecognize?.includes(LLMFactory.PaddleOCR) ||
    layoutRecognize?.toLowerCase()?.includes('paddleocr');

  if (!isPaddleOCRSelected) {
    return null;
  }

  return (
    <div className="space-y-4 border-l-2 border-primary/30 pl-4 ml-2">
      <div className="text-sm font-medium text-text-secondary">
        {t('knowledgeConfiguration.paddleocrOptions', 'PaddleOCR Options')}
      </div>

      <RAGFlowFormItem
        name={buildName('paddleocr_api_url')}
        label={t('knowledgeConfiguration.paddleocrApiUrl', 'PaddleOCR API URL')}
        tooltip={t(
          'knowledgeConfiguration.paddleocrApiUrlTip',
          'The API endpoint URL for PaddleOCR service',
        )}
        horizontal={true}
      >
        {(field) => (
          <Input
            {...field}
            placeholder={t('knowledgeConfiguration.paddleocrApiUrlPlaceholder')}
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('paddleocr_access_token')}
        label={t('knowledgeConfiguration.paddleocrAccessToken', 'AI Studio Access Token')}
        tooltip={t(
          'knowledgeConfiguration.paddleocrAccessTokenTip',
          'Access token for PaddleOCR API (optional)',
        )}
        horizontal={true}
      >
        {(field) => (
          <Input
            {...field}
            placeholder={t('knowledgeConfiguration.paddleocrAccessTokenPlaceholder')}
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('paddleocr_algorithm')}
        label={t('knowledgeConfiguration.paddleocrAlgorithm', 'PaddleOCR Algorithm')}
        tooltip={t(
          'knowledgeConfiguration.paddleocrAlgorithmTip',
          'Algorithm to use for PaddleOCR parsing',
        )}
        horizontal={true}
      >
        {(field) => (
          <RAGFlowSelect
            value={field.value || undefined}
            onChange={field.onChange}
            options={algorithmOptions}
            placeholder={t('common.selectPlaceholder', 'Select value')}
          />
        )}
      </RAGFlowFormItem>
    </div>
  );
}
