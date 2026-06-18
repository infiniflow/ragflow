import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { useListDatasetArtifacts } from '@/hooks/use-dataset-artifact-request';
import { ArtifactListItem } from '@/interfaces/database/dataset-artifact';
import { cn } from '@/lib/utils';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useArtifactSelection } from './hooks/use-artifact-state';

const PAGE_SIZE = 20;

/**
 * Map a stored ``slug_kwd`` (full ``<page_type>/<slug-tail>``) back to its
 * tail. URL selection state stores only the tail because the markdown
 * link format is ``artifact/<kb_id>/<page_type>/<tail>``. Mirrors the
 * helper in artifact-list.tsx — small enough that duplication beats
 * cross-file coupling.
 */
function slugTail(fullSlug: string, pageType: string): string {
  const prefix = `${pageType}/`;
  return fullSlug.startsWith(prefix) ? fullSlug.slice(prefix.length) : fullSlug;
}

function pageTypeBadgeVariant(
  pageType: string,
): 'default' | 'secondary' | 'outline' {
  switch (pageType) {
    case 'entity':
      return 'default';
    case 'concept':
      return 'secondary';
    default:
      return 'outline';
  }
}

/**
 * "Home" view of the dataset Artifact tab — what the middle pane shows
 * when nothing is selected yet. Lists all artifact pages as
 * Google-style result cards (title + page-type badge + summary snippet)
 * ordered by ``outlinks_int DESC`` (most-connected first), paginated 20
 * per page. Clicking a card sets the URL selection, which transitions
 * the middle pane to ``<ArtifactViewer>``.
 */
export function ArtifactNavigatePage() {
  const { t } = useTranslation();
  const { selected, select } = useArtifactSelection();
  const [page, setPage] = useState(1);
  const { data, loading } = useListDatasetArtifacts({
    page,
    pageSize: PAGE_SIZE,
  });

  const items: ArtifactListItem[] = data?.items ?? [];
  const total = data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const isEmpty = !loading && items.length === 0;

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <header className="px-6 pt-6 pb-3 border-b border-border-button">
        <h1 className="text-lg font-semibold text-text-primary">
          {t('artifact.navigate.title')}
        </h1>
        {total > 0 ? (
          <p className="mt-1 text-xs text-text-secondary">
            {t('artifact.navigate.total', { total })}
          </p>
        ) : null}
      </header>

      <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4">
        {loading && items.length === 0 ? (
          <div className="text-sm text-text-secondary">
            {t('common.loading')}
          </div>
        ) : isEmpty ? (
          <div className="text-sm text-text-secondary">
            {t('artifact.navigate.empty')}
          </div>
        ) : (
          <ul className="space-y-4">
            {items.map((item) => {
              const tail = slugTail(item.slug, item.page_type);
              const isSelected =
                selected?.pageType === item.page_type &&
                selected?.slug === tail;
              return (
                <li key={item.slug}>
                  <button
                    type="button"
                    onClick={() =>
                      select({ pageType: item.page_type, slug: tail })
                    }
                    data-selected={isSelected || undefined}
                    className={cn(
                      'w-full text-left p-4 rounded-md border border-border-button',
                      'bg-bg-base hover:bg-bg-card transition-colors',
                      'focus:outline-none focus-visible:ring-1 focus-visible:ring-primary',
                      isSelected && 'bg-bg-card border-primary',
                    )}
                  >
                    <div className="flex items-start gap-2">
                      <h2 className="flex-1 text-sm font-medium text-text-primary line-clamp-1">
                        {item.title}
                      </h2>
                      <Badge
                        variant={pageTypeBadgeVariant(item.page_type)}
                        className="shrink-0"
                      >
                        {item.page_type}
                      </Badge>
                    </div>
                    {item.summary ? (
                      <p className="mt-2 text-sm text-text-secondary line-clamp-2 leading-6">
                        {item.summary}
                      </p>
                    ) : null}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {pageCount > 1 ? (
        <footer className="px-6 py-3 border-t border-border-button flex items-center justify-between">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page <= 1 || loading}
          >
            <ChevronLeft className="size-4" />
            {t('artifact.navigate.prev')}
          </Button>
          <span className="text-xs text-text-secondary">
            {t('artifact.navigate.page', { page, total: pageCount })}
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
            disabled={page >= pageCount || loading}
          >
            {t('artifact.navigate.next')}
            <ChevronRight className="size-4" />
          </Button>
        </footer>
      ) : null}
    </div>
  );
}
