import { Button } from '@/components/ui/button';
import {
  useFetchDatasetArtifactCommit,
  useListDatasetArtifactCommits,
} from '@/hooks/use-dataset-artifact-request';
import { ArtifactCommitListItem } from '@/interfaces/database/dataset-artifact';
import { cn } from '@/lib/utils';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import { History, User as UserIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

dayjs.extend(relativeTime);

interface IProps {
  pageType: string | null;
  slug: string | null;
}

/**
 * Right-side pane on the dataset Artifact tab. Lists the per-page commit
 * history newest-first. Each row is collapsed by default; clicking it
 * lazy-fetches the diff via ``useFetchDatasetArtifactCommit`` and renders
 * an inline unified-diff block. "Load more" simply expands the page size
 * by ``PAGE_SIZE`` — cheap for the small histories we expect.
 */
const PAGE_SIZE = 50;

export function ArtifactCommitsPane({ pageType, slug }: IProps) {
  const { t } = useTranslation();
  const [pageSize, setPageSize] = useState(PAGE_SIZE);
  const [expanded, setExpanded] = useState<string | null>(null);
  const { data, loading } = useListDatasetArtifactCommits(
    pageType ?? undefined,
    slug ?? undefined,
    { pageSize },
  );

  if (!pageType || !slug) return null;

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const isEmpty = !loading && items.length === 0;
  const canLoadMore = items.length < total;

  return (
    <aside className="w-[320px] shrink-0 border-l border-border-button bg-bg-base flex flex-col min-h-0">
      <header className="px-4 py-3 border-b border-border-button flex items-center gap-2">
        <History className="size-4 text-text-secondary" />
        <h3 className="text-sm font-medium text-text-primary">
          {t('artifact.history.title')}
        </h3>
        {total > 0 ? (
          <span className="ml-auto text-xs text-text-secondary">{total}</span>
        ) : null}
      </header>

      <div className="flex-1 min-h-0 overflow-y-auto">
        {isEmpty ? (
          <div className="px-4 py-6 text-sm text-text-secondary">
            {t('artifact.history.empty')}
          </div>
        ) : (
          <ul className="divide-y divide-border-button">
            {items.map((c) => (
              <CommitRow
                key={c.id}
                item={c}
                expanded={expanded === c.id}
                onToggle={() => setExpanded(expanded === c.id ? null : c.id)}
              />
            ))}
          </ul>
        )}

        {loading && items.length === 0 ? (
          <div className="px-4 py-4 text-xs text-text-secondary">
            {t('common.loading')}
          </div>
        ) : null}

        {canLoadMore ? (
          <div className="p-3">
            <Button
              variant="ghost"
              size="sm"
              className="w-full"
              onClick={() => setPageSize((s) => s + PAGE_SIZE)}
              disabled={loading}
            >
              {t('artifact.history.loadMore')}
            </Button>
          </div>
        ) : null}
      </div>
    </aside>
  );
}

function CommitRow({
  item,
  expanded,
  onToggle,
}: {
  item: ArtifactCommitListItem;
  expanded: boolean;
  onToggle: () => void;
}) {
  const author = item.user_nickname || item.user_id || '';
  const rel = item.create_time ? dayjs(item.create_time).fromNow() : '';

  return (
    <li>
      <button
        type="button"
        onClick={onToggle}
        className={cn(
          'w-full text-left px-4 py-3 hover:bg-bg-card transition-colors',
          'focus:outline-none focus-visible:ring-1 focus-visible:ring-primary',
          expanded && 'bg-bg-card',
        )}
      >
        <div className="text-sm text-text-primary line-clamp-2">
          {item.title}
        </div>
        <div className="mt-1 flex items-center gap-2 text-xs text-text-secondary">
          {author ? (
            <span className="inline-flex items-center gap-1 truncate max-w-[8rem]">
              <UserIcon className="size-3 shrink-0" />
              <span className="truncate">{author}</span>
            </span>
          ) : null}
          <span className="ml-auto whitespace-nowrap">{rel}</span>
        </div>
        {item.comments ? (
          <div className="mt-1 text-xs text-text-secondary line-clamp-2 whitespace-pre-wrap">
            {item.comments}
          </div>
        ) : null}
      </button>
      {expanded ? <CommitDiff commitId={item.id} /> : null}
    </li>
  );
}

function CommitDiff({ commitId }: { commitId: string }) {
  const { t } = useTranslation();
  const { data, loading } = useFetchDatasetArtifactCommit(commitId);

  if (loading && !data) {
    return (
      <div className="px-4 py-2 text-xs text-text-secondary">
        {t('common.loading')}
      </div>
    );
  }
  const diff = data?.diff || '';
  if (!diff) {
    return (
      <div className="px-4 py-2 text-xs text-text-secondary">
        {t('artifact.history.noDiff')}
      </div>
    );
  }

  // Tiny inline renderer: color +/-/@ lines via Tailwind so we don't pull
  // in a heavyweight diff component just for v1.
  const lines = diff.split('\n');
  return (
    <pre className="px-4 py-2 text-[11px] leading-5 font-mono bg-bg-card overflow-x-auto">
      {lines.map((ln, i) => {
        let cls = 'text-text-secondary';
        if (ln.startsWith('+++') || ln.startsWith('---')) {
          cls = 'text-text-primary font-semibold';
        } else if (ln.startsWith('+')) {
          cls = 'text-success';
        } else if (ln.startsWith('-')) {
          cls = 'text-destructive';
        } else if (ln.startsWith('@@')) {
          cls = 'text-accent-primary';
        }
        return (
          <div key={i} className={cls}>
            {ln || ' '}
          </div>
        );
      })}
    </pre>
  );
}
