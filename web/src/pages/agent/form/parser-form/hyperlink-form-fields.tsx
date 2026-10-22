import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { HyperlinkLimits } from '../../constant/pipeline';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

type HyperlinkFormFieldsProps = CommonProps & {
  // collect_markdown_links is rejected by the backend outside the markdown
  // family, so only the markdown form may render the switch.
  showMarkdownLinks?: boolean;
};

type HyperlinkSwitchProps = CommonProps & {
  name: string;
  label: string;
  tooltip: string;
};

function HyperlinkSwitchFormField({
  prefix,
  name,
  label,
  tooltip,
}: HyperlinkSwitchProps) {
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(name, prefix)}
      label={label}
      tooltip={tooltip}
      horizontal={true}
      labelClassName="w-full"
      valueClassName="w-8"
    >
      {(field) => (
        <Switch
          checked={!!field.value}
          onCheckedChange={(checked) => field.onChange?.(checked)}
        />
      )}
    </RAGFlowFormItem>
  );
}

type HyperlinkBudgetProps = CommonProps & {
  name: string;
  label: string;
  tooltip: string;
  max: number;
};

function HyperlinkBudgetFormField({
  prefix,
  name,
  label,
  tooltip,
  max,
}: HyperlinkBudgetProps) {
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(name, prefix)}
      label={label}
      tooltip={tooltip}
    >
      <Input type="number" min={1} max={max} step={1} />
    </RAGFlowFormItem>
  );
}

export function HyperlinkFormFields({
  prefix,
  showMarkdownLinks = false,
}: HyperlinkFormFieldsProps) {
  const { t } = useTranslation();
  const enabled = useWatch({
    name: buildFieldNameWithPrefix('fetch_links', prefix),
  });

  return (
    <>
      <HyperlinkSwitchFormField
        prefix={prefix}
        name="fetch_links"
        label={t('flow.fetchLinks')}
        tooltip={t('flow.fetchLinksTip')}
      />
      {enabled && (
        <div className="space-y-4">
          {showMarkdownLinks && (
            <HyperlinkSwitchFormField
              prefix={prefix}
              name="collect_markdown_links"
              label={t('flow.collectMarkdownLinks')}
              tooltip={t('flow.collectMarkdownLinksTip')}
            />
          )}
          <HyperlinkBudgetFormField
            prefix={prefix}
            name="max_urls"
            label={t('flow.hyperlinkMaxUrls')}
            tooltip={t('flow.hyperlinkMaxUrlsTip')}
            max={HyperlinkLimits.maxUrls}
          />
          <HyperlinkBudgetFormField
            prefix={prefix}
            name="max_bytes_per_url"
            label={t('flow.hyperlinkMaxBytesPerUrl')}
            tooltip={t('flow.hyperlinkMaxBytesPerUrlTip')}
            max={HyperlinkLimits.maxBytesPerUrl}
          />
          <HyperlinkBudgetFormField
            prefix={prefix}
            name="max_total_bytes"
            label={t('flow.hyperlinkMaxTotalBytes')}
            tooltip={t('flow.hyperlinkMaxTotalBytesTip')}
            max={HyperlinkLimits.maxTotalBytes}
          />
          <HyperlinkBudgetFormField
            prefix={prefix}
            name="timeout_s"
            label={t('flow.hyperlinkTimeout')}
            tooltip={t('flow.hyperlinkTimeoutTip')}
            max={HyperlinkLimits.timeoutSeconds}
          />
        </div>
      )}
    </>
  );
}
