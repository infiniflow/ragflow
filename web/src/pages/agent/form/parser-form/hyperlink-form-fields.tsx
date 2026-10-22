import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Switch } from '@/components/ui/switch';
import { Input } from '@/components/ui/input';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

type HyperlinkFormFieldsProps = CommonProps & {
  // collect_markdown_links is rejected by the backend outside the markdown
  // family, so only the markdown form may render the switch.
  showMarkdownLinks?: boolean;
};

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
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix('fetch_links', prefix)}
        label={t('flow.fetchLinks')}
        tooltip={t('flow.fetchLinksTip')}
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
      {enabled && (
        <div className="space-y-4">
          {showMarkdownLinks && (
            <RAGFlowFormItem
              name={buildFieldNameWithPrefix('collect_markdown_links', prefix)}
              label={t('flow.collectMarkdownLinks')}
              tooltip={t('flow.collectMarkdownLinksTip')}
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
          )}
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix('max_urls', prefix)}
            label={t('flow.hyperlinkMaxUrls')}
            description={t('flow.hyperlinkMaxUrlsTip')}
          >
            <Input type="number" min={1} max={50} step={1} />
          </RAGFlowFormItem>
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix('max_bytes_per_url', prefix)}
            label={t('flow.hyperlinkMaxBytesPerUrl')}
          >
            <Input type="number" min={1} max={64 * 1024 * 1024} step={1} />
          </RAGFlowFormItem>
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix('max_total_bytes', prefix)}
            label={t('flow.hyperlinkMaxTotalBytes')}
          >
            <Input type="number" min={1} max={256 * 1024 * 1024} step={1} />
          </RAGFlowFormItem>
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix('timeout_s', prefix)}
            label={t('flow.hyperlinkTimeout')}
          >
            <Input type="number" min={1} max={120} step={1} />
          </RAGFlowFormItem>
        </div>
      )}
    </>
  );
}
