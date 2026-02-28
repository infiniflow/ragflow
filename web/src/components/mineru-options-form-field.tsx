import { RAGFlowFormItem } from '@/components/ragflow-form';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Input } from '@/components/ui/input';
import { LLMFactory } from '@/constants/llm';
import { buildOptions } from '@/utils/form';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

const parseMethodOptions = buildOptions(['auto', 'txt', 'ocr']);
const languageOptions = buildOptions([
  'English',
  'Chinese',
  'Traditional Chinese',
  'Russian',
  'Ukrainian',
  'Indonesian',
  'Spanish',
  'Vietnamese',
  'Japanese',
  'Korean',
  'Portuguese BR',
  'German',
  'French',
  'Italian',
  'Tamil',
  'Telugu',
  'Kannada',
  'Thai',
  'Greek',
  'Hindi',
]);

// MinerU backend options
const backendOptions = buildOptions([
  'hybrid-auto-engine',
  'pipeline',
  'vlm-auto-engine',
  'vlm-http-client',
  'hybrid-http-client',
]);

export function MinerUOptionsFormField({
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

  // Check if MinerU is selected (the value contains 'MinerU' or matches the factory name)
  const isMinerUSelected =
    layoutRecognize?.includes(LLMFactory.MinerU) ||
    layoutRecognize?.toLowerCase()?.includes('mineru');

  if (!isMinerUSelected) {
    return null;
  }

  return (
    <div className="space-y-4 border-l-2 border-primary/30 pl-4 ml-2">
      <div className="text-sm font-medium text-text-secondary">
        {t('knowledgeConfiguration.mineruOptions', 'MinerU Options')}
      </div>

      <RAGFlowFormItem
        name={buildName('mineru_backend')}
        label={t('knowledgeConfiguration.mineruBackend', 'Backend')}
        tooltip={t(
          'knowledgeConfiguration.mineruBackendTip',
          'MinerU processing backend. hybrid-auto-engine is recommended.',
        )}
        horizontal={true}
      >
        {(field) => (
          <RAGFlowSelect
            value={field.value || 'hybrid-auto-engine'}
            onChange={field.onChange}
            options={backendOptions}
            placeholder={t('common.selectPlaceholder', 'Select value')}
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('mineru_parse_method')}
        label={t('knowledgeConfiguration.mineruParseMethod', 'Parse Method')}
        tooltip={t(
          'knowledgeConfiguration.mineruParseMethodTip',
          'Method for parsing PDF: auto (automatic detection), txt (text extraction), ocr (optical character recognition)',
        )}
        horizontal={true}
      >
        {(field) => (
          <RAGFlowSelect
            value={field.value || 'auto'}
            onChange={field.onChange}
            options={parseMethodOptions}
            placeholder={t('common.selectPlaceholder', 'Select value')}
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('mineru_lang')}
        label={t('knowledgeConfiguration.mineruLanguage', 'Language')}
        tooltip={t(
          'knowledgeConfiguration.mineruLanguageTip',
          'Preferred OCR language for MinerU.',
        )}
        horizontal={true}
      >
        {(field) => (
          <RAGFlowSelect
            value={field.value || 'English'}
            onChange={field.onChange}
            options={languageOptions}
            placeholder={t('common.selectPlaceholder', 'Select value')}
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('mineru_formula_enable')}
        label={t(
          'knowledgeConfiguration.mineruFormulaEnable',
          'Formula Recognition',
        )}
        tooltip={t(
          'knowledgeConfiguration.mineruFormulaEnableTip',
          'Enable formula recognition. Note: This may not work correctly for Cyrillic documents.',
        )}
        horizontal={true}
        labelClassName="!mb-0"
      >
        {(field) => (
          <Switch
            checked={field.value ?? true}
            onCheckedChange={field.onChange}
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('mineru_table_enable')}
        label={t(
          'knowledgeConfiguration.mineruTableEnable',
          'Table Recognition',
        )}
        tooltip={t(
          'knowledgeConfiguration.mineruTableEnableTip',
          'Enable table recognition and extraction.',
        )}
        horizontal={true}
        labelClassName="!mb-0"
      >
        {(field) => (
          <Switch
            checked={field.value ?? true}
            onCheckedChange={field.onChange}
          />
        )}
      </RAGFlowFormItem>

      <div className="text-sm font-medium text-text-secondary mt-4">
        {t('knowledgeConfiguration.mineruBatchOptions', 'Batch Processing Options')}
      </div>

      <RAGFlowFormItem
        name={buildName('mineru_batch_size')}
        label={t('knowledgeConfiguration.mineruBatchSize', 'Batch Size')}
        tooltip={t(
          'knowledgeConfiguration.mineruBatchSizeTip',
          'Number of pages to process per batch for large PDFs. Default is 30.',
        )}
        horizontal={true}
      >
        {(field) => (
          <Input
            type="number"
            min={1}
            max={500}
            value={field.value ?? 30}
            onChange={(e) => {
              const parsed = parseInt(e.target.value, 10);
              field.onChange(Number.isNaN(parsed) ? 30 : parsed);
            }}
            placeholder="30"
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('mineru_start_page')}
        label={t('knowledgeConfiguration.mineruStartPage', 'Start Page')}
        tooltip={t(
          'knowledgeConfiguration.mineruStartPageTip',
          'Starting page number for processing (0-based). Leave empty to start from beginning.',
        )}
        horizontal={true}
      >
        {(field) => (
          <Input
            type="number"
            min={0}
            value={field.value ?? ''}
            onChange={(e) => field.onChange(e.target.value ? parseInt(e.target.value, 10) : null)}
            placeholder="Optional"
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('mineru_end_page')}
        label={t('knowledgeConfiguration.mineruEndPage', 'End Page')}
        tooltip={t(
          'knowledgeConfiguration.mineruEndPageTip',
          'Ending page number for processing (0-based, inclusive). Leave empty to process until end.',
        )}
        horizontal={true}
      >
        {(field) => (
          <Input
            type="number"
            min={0}
            value={field.value ?? ''}
            onChange={(e) => field.onChange(e.target.value ? parseInt(e.target.value, 10) : null)}
            placeholder="Optional"
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('mineru_strict_mode')}
        label={t(
          'knowledgeConfiguration.mineruStrictMode',
          'Strict Mode',
        )}
        tooltip={t(
          'knowledgeConfiguration.mineruStrictModeTip',
          'If enabled, all batches must succeed. If disabled, partial success is allowed with warnings.',
        )}
        horizontal={true}
        labelClassName="!mb-0"
      >
        {(field) => (
          <Switch
            checked={field.value ?? true}
            onCheckedChange={field.onChange}
          />
        )}
      </RAGFlowFormItem>
    </div>
  );
}
