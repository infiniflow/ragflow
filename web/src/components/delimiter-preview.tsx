import { useTranslation } from 'react-i18next';

import { Badge } from './ui/badge';
import { parseDelimitersForDisplay } from '@/utils/delimiter-preview';

interface Props {
  /** The current value of the delimiter field. */
  value: string | undefined;
}

/**
 * Renders a best-effort, informational preview of the delimiters
 * derived from `value`. Pure display — does not affect the stored value
 * or the chunking behavior. The preview may differ from what the
 * backend actually parses (see #17383).
 *
 * Each delimiter is shown as a small monospace badge; whitespace
 * characters (which would otherwise be invisible in the single-line
 * input) are rendered with visible Unicode glyphs (`↵` for newline,
 * `⇥` for tab, `␣` for space, etc.).
 */
export function DelimiterPreview({ value }: Props) {
  const { t } = useTranslation();
  const parsed = parseDelimitersForDisplay(value);

  if (parsed.length === 0) {
    return (
      <p
        className="pt-1 text-xs italic text-text-secondary"
        data-testid="delimiter-preview-empty"
      >
        {t('knowledgeDetails.delimiterPreviewEmpty', {
          defaultValue: 'No delimiters — text will be chunked by size only.',
        })}
      </p>
    );
  }

  return (
    <div
      className="flex flex-wrap items-center gap-1 pt-1"
      data-testid="delimiter-preview"
    >
      <span className="text-xs text-text-secondary">
        {t('knowledgeDetails.delimiterPreviewLabel', {
          defaultValue: 'Splits at:',
        })}
      </span>
      {parsed.map((d, i) => (
        <Badge
          key={`${d.display}-${i}`}
          variant="secondary"
          className="font-mono text-xs"
        >
          {d.display}
        </Badge>
      ))}
      <span className="text-xs text-text-secondary">
        {t('knowledgeDetails.delimiterPreviewCount', {
          count: parsed.length,
          defaultValue: '({{count}})',
        }).replace('{{count}}', String(parsed.length))}
      </span>
    </div>
  );
}
