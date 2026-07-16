import { RAGFlowFormItem } from '@/components/ragflow-form';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { LLMFactory } from '@/constants/llm';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

const hybridOptions = [
  { label: 'Off (local parsing only)', value: 'off' },
  { label: 'docling-fast', value: 'docling-fast' },
  { label: 'hancom-ai', value: 'hancom-ai' },
];

const hybridModeOptions = [
  { label: 'Auto (per-page triage)', value: 'auto' },
  { label: 'Full (force every page)', value: 'full' },
];

export function OpenDataLoaderOptionsFormField({
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

  const hybrid = useWatch({
    control: form.control,
    name: buildName('hybrid'),
  });

  const isOpenDataLoaderSelected =
    layoutRecognize?.includes(LLMFactory.OpenDataLoader) ||
    layoutRecognize?.toLowerCase()?.includes('opendataloader');

  if (!isOpenDataLoaderSelected) {
    return null;
  }

  const isHybridActive = !!hybrid && hybrid !== 'off';

  return (
    <div className="space-y-4 border-l-2 border-primary/30 pl-4 ml-2">
      <div className="text-sm font-medium text-text-secondary">
        {t('knowledgeConfiguration.openDataLoaderOptions', 'OpenDataLoader Options')}
      </div>

      <RAGFlowFormItem
        name={buildName('hybrid')}
        label={t('knowledgeConfiguration.openDataLoaderHybrid', 'Hybrid backend')}
        tooltip={t(
          'knowledgeConfiguration.openDataLoaderHybridTip',
          'Route complex pages (borderless tables, scanned images) to an AI-assisted backend instead of the deterministic local parser. Off = local parsing only.',
        )}
        horizontal={true}
      >
        {(field) => (
          <RAGFlowSelect
            value={field.value || 'off'}
            onChange={field.onChange}
            options={hybridOptions}
            placeholder={t('common.selectPlaceholder', 'Select value')}
          />
        )}
      </RAGFlowFormItem>

      {isHybridActive && (
        <RAGFlowFormItem
          name={buildName('hybrid_mode')}
          label={t(
            'knowledgeConfiguration.openDataLoaderHybridMode',
            'Hybrid mode',
          )}
          tooltip={t(
            'knowledgeConfiguration.openDataLoaderHybridModeTip',
            'Only applies when a hybrid backend is selected above. Auto lets the parser decide per page which pages need the hybrid backend. Full forces every page through it — use for scanned/image-only documents where OCR must run on the whole document.',
          )}
          horizontal={true}
        >
          {(field) => (
            <RAGFlowSelect
              value={field.value || 'auto'}
              onChange={field.onChange}
              options={hybridModeOptions}
              placeholder={t('common.selectPlaceholder', 'Select value')}
            />
          )}
        </RAGFlowFormItem>
      )}

      <RAGFlowFormItem
        name={buildName('sanitize')}
        label={t('knowledgeConfiguration.openDataLoaderSanitize', 'Sanitize output')}
        tooltip={t(
          'knowledgeConfiguration.openDataLoaderSanitizeTip',
          'Replace emails, phone numbers, IPs, credit card numbers, and URLs found in the document with placeholders.',
        )}
        horizontal={true}
        labelClassName="!mb-0"
      >
        {(field) => (
          <Switch checked={field.value ?? false} onCheckedChange={field.onChange} />
        )}
      </RAGFlowFormItem>
    </div>
  );
}
