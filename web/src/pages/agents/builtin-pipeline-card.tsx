import { HomeCard } from '@/components/home-card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { IBuiltinPipelineListItem } from '@/interfaces/database/agent';
import { Copy } from 'lucide-react';
import type { MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useCopyBuiltinPipeline } from './use-copy-builtin-pipeline';

// BuiltinPipelineCard renders a read-only built-in ingestion pipeline in the
// agents list. Built-in pipelines are static, non-user resources: they cannot
// be opened, renamed, or deleted, but can be copied into a user-owned dataflow
// canvas via the Copy action.
export function BuiltinPipelineCard({
  data,
}: {
  data: IBuiltinPipelineListItem;
}) {
  const { t } = useTranslation();
  const { copy, copying } = useCopyBuiltinPipeline();

  // The built-in marker is rendered as a badge. Reuse the existing
  // " (built in)" suffix but drop the surrounding parentheses for a cleaner
  // badge label, falling back to a plain label if the locale is missing it.
  const builtInLabel = (
    t('knowledgeConfiguration.builtInSuffix') || ' (built in)'
  )
    .trim()
    .replace(/^\(|\)$/g, '');

  return (
    <HomeCard
      testId="builtin-pipeline-card"
      data={{
        name: data.title ?? '',
        description: data.description || '',
      }}
      // Read-only: a built-in pipeline cannot be opened for editing.
      onClick={undefined}
      // No rename / delete / edit-tags menu for built-in items.
      moreDropdown={null}
      icon={
        <Badge variant="secondary" data-testid="builtin-badge">
          {builtInLabel}
        </Badge>
      }
      extra={
        <Button
          data-testid="copy-builtin-pipeline"
          variant="static"
          size="auto"
          disabled={copying}
          onClick={(e: MouseEvent) => {
            e.stopPropagation();
            copy({ id: data.id, title: data.title ?? '' });
          }}
        >
          <Copy className="size-[1em]" />
          {t('common.copy')}
        </Button>
      }
    />
  );
}
