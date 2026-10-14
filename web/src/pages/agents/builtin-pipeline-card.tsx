import { HomeCard } from '@/components/home-card';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IBuiltinPipelineListItem } from '@/interfaces/database/agent';
import { Copy } from 'lucide-react';
import type { MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useCopyBuiltinPipeline } from './use-copy-builtin-pipeline';

// BuiltinPipelineCard renders a read-only built-in ingestion pipeline in the
// agents list. Built-in pipelines are static, non-user resources: clicking the
// card opens them on the read-only canvas (view only, no editing or autosave),
// and the Copy action turns them into a user-owned editable dataflow canvas.
export function BuiltinPipelineCard({
  data,
}: {
  data: IBuiltinPipelineListItem;
}) {
  const { t } = useTranslation();
  const { copy, copying } = useCopyBuiltinPipeline();
  const { navigateToBuiltinPipeline } = useNavigatePage();

  const handleCopyClick = (e: MouseEvent) => {
    e.stopPropagation();
    copy({ id: data.id, title: data.title ?? '' });
  };

  return (
    <HomeCard
      testId="builtin-pipeline-card"
      data={{
        name: data.title ?? '',
        description: data.description || '',
      }}
      // Read-only: clicking opens the built-in pipeline on the view-only
      // canvas; the template itself can never be edited.
      onClick={navigateToBuiltinPipeline(data.id)}
      // No rename / delete / edit-tags menu for built-in items; the top-right
      // slot carries the icon-only Copy action instead.
      moreDropdown={
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              data-testid="copy-builtin-pipeline"
              variant="static"
              size="icon-sm"
              disabled={copying}
              aria-label={t('common.copy')}
              onClick={handleCopyClick}
            >
              <Copy className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('common.copy')}</TooltipContent>
        </Tooltip>
      }
    />
  );
}
